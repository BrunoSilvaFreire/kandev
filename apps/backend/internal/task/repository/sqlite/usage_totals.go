package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// costSourceUnpriced mirrors internal/task/usage.CostSourceUnpriced. It is
// redeclared here rather than imported: internal/task/usage already imports
// this package to satisfy its Repository interface, so importing back would
// cycle.
const costSourceUnpriced = "unpriced"

// usageTotalsAggregateColumns is AC-18/AC-19's read-side aggregation, shared
// verbatim by the single-scope totals query and the grouped breakdown query.
// Every sum is order-independent (SUM, COUNT, MIN, MAX), so it needs none of
// ListTaskUsageEvents' (occurred_at, id) ordering (AC-16). A nullable token
// column is coalesced to zero before summing (AC-12) so one not-recorded
// sample can never null out the whole total. tokens_total sums the stored
// per-row column verbatim - it is never recomputed from the per-kind sums at
// read time (AC-19). The `?` binds the unpriced cost source; it is always the
// first query parameter.
const usageTotalsAggregateColumns = `
		COUNT(*),
		COALESCE(SUM(tokens_in), 0),
		COALESCE(SUM(COALESCE(tokens_cached_read, 0)), 0),
		COALESCE(SUM(COALESCE(tokens_cached_write, 0)), 0),
		COALESCE(SUM(COALESCE(tokens_out, 0)), 0),
		COALESCE(SUM(COALESCE(tokens_thought, 0)), 0),
		COALESCE(SUM(tokens_total), 0),
		COALESCE(SUM(cost_subcents), 0),
		COALESCE(SUM(CASE WHEN estimated = 1 THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN cost_source = ? THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN tokens_out IS NULL THEN 1 ELSE 0 END), 0),
		MIN(occurred_at),
		MAX(occurred_at)`

// usageTotalsAggregateQuery is the single-scope aggregate. %s is one of the
// two internal scope columns (never caller input).
const usageTotalsAggregateQuery = `
	SELECT` + usageTotalsAggregateColumns + `
	  FROM task_usage_events
	 WHERE %s = ?
`

// usageTotalsGroupedQuery aggregates per (session, agent profile, agent type,
// model, provider), newest activity first. It is plain portable GROUP BY, with
// no dialect branch. The five leading columns are scanned before the shared
// aggregate columns.
const usageTotalsGroupedQuery = `
	SELECT
		session_id,
		agent_profile_id,
		agent_type,
		model,
		provider,` + usageTotalsAggregateColumns + `
	  FROM task_usage_events
	 WHERE task_id = ?
	 GROUP BY session_id, agent_profile_id, agent_type, model, provider
	 ORDER BY MAX(occurred_at) DESC
`

// GetTaskUsageTotals aggregates every ledger row for taskID, including rows
// whose session_id has been cleared by session deletion (AC-18, AC-19).
// An unknown task, and a known task with no rows, both return a zeroed
// TaskUsageTotals and a nil error (AC-20) - the caller enforces the 404 for
// an unknown task, since the aggregate query alone cannot distinguish the
// two.
func (r *Repository) GetTaskUsageTotals(ctx context.Context, taskID string) (*models.TaskUsageTotals, error) {
	return r.scanUsageTotals(ctx, "task_id", taskID)
}

// GetSessionUsageTotals aggregates only the ledger rows still bound to
// sessionID (AC-18, AC-19). A session with no rows returns a zeroed
// TaskUsageTotals and a nil error (AC-20).
func (r *Repository) GetSessionUsageTotals(ctx context.Context, sessionID string) (*models.TaskUsageTotals, error) {
	return r.scanUsageTotals(ctx, "session_id", sessionID)
}

// usageRowScanner is satisfied by *sql.Row and *sqlx.Rows; scanUsageTotalsRow
// only needs Scan.
type usageRowScanner interface {
	Scan(dest ...any) error
}

// scanUsageTotalsRow reads the shared aggregate columns into a TaskUsageTotals
// and applies the post-processing the SQL cannot (unpriced count,
// OutputTokensComplete, both timestamps). leading holds scan targets for any
// columns selected before the aggregate list (the grouped query's five group
// keys); it is empty for the single-scope query.
func scanUsageTotalsRow(scanner usageRowScanner, leading ...any) (*models.TaskUsageTotals, error) {
	var (
		totals                     models.TaskUsageTotals
		rawFirstEventAt            interface{}
		rawLastEventAt             interface{}
		unpricedEventCount         sql.NullInt64
		outputIncompleteEventCount sql.NullInt64
	)
	dest := make([]any, 0, len(leading)+13)
	dest = append(dest, leading...)
	dest = append(dest,
		&totals.EventCount,
		&totals.TokensIn,
		&totals.TokensCachedRead,
		&totals.TokensCachedWrite,
		&totals.TokensOut,
		&totals.TokensThought,
		&totals.TokensTotal,
		&totals.CostSubcents,
		&totals.EstimatedEventCount,
		&unpricedEventCount,
		&outputIncompleteEventCount,
		&rawFirstEventAt,
		&rawLastEventAt,
	)
	if err := scanner.Scan(dest...); err != nil {
		return nil, err
	}

	totals.UnpricedEventCount = unpricedEventCount.Int64
	totals.OutputTokensComplete = outputIncompleteEventCount.Int64 == 0

	firstEventAt, err := parseUsageTotalsTimestamp(rawFirstEventAt)
	if err != nil {
		return nil, fmt.Errorf("parse first_event_at: %w", err)
	}
	totals.FirstEventAt = firstEventAt
	lastEventAt, err := parseUsageTotalsTimestamp(rawLastEventAt)
	if err != nil {
		return nil, fmt.Errorf("parse last_event_at: %w", err)
	}
	totals.LastEventAt = lastEventAt

	return &totals, nil
}

// scanUsageTotals runs usageTotalsAggregateQuery scoped to one column.
// scopeColumn is always one of the two internal literals passed by
// GetTaskUsageTotals/GetSessionUsageTotals, never caller input, so
// interpolating it into the query text carries no injection risk.
func (r *Repository) scanUsageTotals(ctx context.Context, scopeColumn, scopeValue string) (*models.TaskUsageTotals, error) {
	if scopeColumn != "task_id" && scopeColumn != "session_id" {
		return nil, fmt.Errorf("scanUsageTotals: unsupported scope column %q", scopeColumn)
	}
	query := r.ro.Rebind(fmt.Sprintf(usageTotalsAggregateQuery, scopeColumn))
	row := r.ro.QueryRowxContext(ctx, query, costSourceUnpriced, scopeValue)
	return scanUsageTotalsRow(row)
}

// ListTaskUsageTotalGroups aggregates the task's ledger rows at the finest
// grain (session, agent profile, agent type, model, provider), newest activity
// first. Rows whose session was deleted carry a nil SessionID. An unknown task
// or a task with no rows returns an empty slice and a nil error; the caller
// enforces the 404 for an unknown task.
func (r *Repository) ListTaskUsageTotalGroups(ctx context.Context, taskID string) ([]*models.TaskUsageTotalsGroup, error) {
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(usageTotalsGroupedQuery), costSourceUnpriced, taskID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	groups := make([]*models.TaskUsageTotalsGroup, 0)
	for rows.Next() {
		var (
			sessionID      sql.NullString
			agentProfileID string
			agentType      string
			model          string
			provider       string
		)
		totals, err := scanUsageTotalsRow(
			rows, &sessionID, &agentProfileID, &agentType, &model, &provider,
		)
		if err != nil {
			return nil, err
		}
		group := &models.TaskUsageTotalsGroup{
			AgentProfileID: agentProfileID,
			AgentType:      agentType,
			Model:          model,
			Provider:       provider,
			Totals:         *totals,
		}
		if sessionID.Valid {
			id := sessionID.String
			group.SessionID = &id
		}
		groups = append(groups, group)
	}
	return groups, rows.Err()
}

// parseUsageTotalsTimestamp converts a MIN/MAX(occurred_at) scan result to a
// *time.Time, or nil for SQL NULL (no contributing rows). Scanned as
// interface{} because SQLite returns an aggregate over a TIMESTAMP column as
// TEXT/[]byte while PostgreSQL returns a native time.Time; reuses
// parseTaskActivityTime (task_status_summary.go), which already handles both.
func parseUsageTotalsTimestamp(raw interface{}) (*time.Time, error) {
	if raw == nil {
		return nil, nil
	}
	parsed, err := parseTaskActivityTime(raw)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}
