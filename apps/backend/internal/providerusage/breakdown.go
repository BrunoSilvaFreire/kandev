package providerusage

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/db/dialect"
)

// Breakdown group-by dimensions.
const (
	BreakdownGroupSession  = "session"
	BreakdownGroupTask     = "task"
	BreakdownGroupModel    = "model"
	BreakdownGroupProvider = "provider"
	BreakdownGroupAgent    = "agent"
	BreakdownGroupDay      = "day"
)

// Breakdown sort columns.
const (
	BreakdownSortTokens = "tokens"
	BreakdownSortCost   = "cost"
	BreakdownSortEvents = "events"
	BreakdownSortLastAt = "last_at"
)

// Breakdown order directions.
const (
	BreakdownOrderAsc  = "asc"
	BreakdownOrderDesc = "desc"
)

const (
	breakdownDefaultLimit = 50
	breakdownMaxLimit     = 200
	breakdownMaxQueryLen  = 200
)

var breakdownGroups = map[string]bool{
	BreakdownGroupSession:  true,
	BreakdownGroupTask:     true,
	BreakdownGroupModel:    true,
	BreakdownGroupProvider: true,
	BreakdownGroupAgent:    true,
	BreakdownGroupDay:      true,
}

var breakdownSorts = map[string]bool{
	BreakdownSortTokens: true,
	BreakdownSortCost:   true,
	BreakdownSortEvents: true,
	BreakdownSortLastAt: true,
}

var breakdownOrders = map[string]bool{
	BreakdownOrderAsc:  true,
	BreakdownOrderDesc: true,
}

// BreakdownQuery is the validated request for the grouped ledger breakdown.
// Every enum-like field is narrowed to a closed whitelist by normalize.
type BreakdownQuery struct {
	Range     string
	GroupBy   string
	Provider  string
	Model     string
	AgentType string
	TaskID    string
	SessionID string
	Q         string
	Sort      string
	Order     string
	Limit     int
	Offset    int
}

// normalize fills defaults, clamps pagination, and rejects unknown enum
// values so a caller can never reach the SQL builders with an arbitrary value.
func (q *BreakdownQuery) normalize() error {
	if q.GroupBy == "" {
		q.GroupBy = BreakdownGroupSession
	}
	if !breakdownGroups[q.GroupBy] {
		return &breakdownValidationError{message: fmt.Sprintf("unknown group_by %q", q.GroupBy)}
	}
	if q.Sort == "" {
		q.Sort = BreakdownSortTokens
	}
	if !breakdownSorts[q.Sort] {
		return &breakdownValidationError{message: fmt.Sprintf("unknown sort %q", q.Sort)}
	}
	if q.Order == "" {
		q.Order = BreakdownOrderDesc
	}
	if !breakdownOrders[q.Order] {
		return &breakdownValidationError{message: fmt.Sprintf("unknown order %q", q.Order)}
	}
	if q.Limit <= 0 {
		q.Limit = breakdownDefaultLimit
	}
	if q.Limit > breakdownMaxLimit {
		q.Limit = breakdownMaxLimit
	}
	if q.Offset < 0 {
		q.Offset = 0
	}
	q.Q = strings.TrimSpace(q.Q)
	if len(q.Q) > breakdownMaxQueryLen {
		q.Q = q.Q[:breakdownMaxQueryLen]
	}
	return nil
}

// BreakdownRow is one grouped ledger aggregate.
type BreakdownRow struct {
	Key              string    `json:"key"`
	Label            string    `json:"label"`
	TaskID           string    `json:"task_id"`
	SessionName      string    `json:"session_name"`
	Provider         string    `json:"provider"`
	Model            string    `json:"model"`
	Models           int64     `json:"models"`
	TokensTotal      int64     `json:"tokens_total"`
	TokensIn         int64     `json:"tokens_in"`
	TokensOut        int64     `json:"tokens_out"`
	TokensCachedRead int64     `json:"tokens_cached_read"`
	CostSubcents     int64     `json:"cost_subcents"`
	Events           int64     `json:"events"`
	FirstAt          time.Time `json:"first_at"`
	LastAt           time.Time `json:"last_at"`
}

// BreakdownFacets are the distinct filter values available for the range.
type BreakdownFacets struct {
	Providers  []string `json:"providers"`
	Models     []string `json:"models"`
	AgentTypes []string `json:"agent_types"`
}

// BreakdownResponse is the GET /api/v1/provider-usage/breakdown payload.
type BreakdownResponse struct {
	Range             string          `json:"range"`
	GroupBy           string          `json:"group_by"`
	TotalRows         int64           `json:"total_rows"`
	TotalTokens       int64           `json:"total_tokens"`
	TotalCostSubcents int64           `json:"total_cost_subcents"`
	TotalEvents       int64           `json:"total_events"`
	Facets            BreakdownFacets `json:"facets"`
	Rows              []BreakdownRow  `json:"rows"`
}

// providerCaseExpr mirrors ledgerProvider in SQL so the breakdown groups and
// filters attribute rows to the same provider the backfill records.
func providerCaseExpr() string {
	return "CASE e.agent_type" +
		" WHEN '" + AgentTypeClaude + "' THEN '" + ProviderAnthropic + "'" +
		" WHEN '" + AgentTypeCodex + "' THEN '" + ProviderOpenAI + "'" +
		" WHEN '" + AgentTypeAgy + "' THEN '" + ProviderAntigravity + "'" +
		" WHEN '" + AgentTypeAntigravity + "' THEN '" + ProviderAntigravity + "'" +
		" ELSE COALESCE(NULLIF(e.provider, ''), e.agent_type) END"
}

// breakdownGroupSpec is the SQL projection for one group-by dimension.
type breakdownGroupSpec struct {
	keyExpr    string
	labelExpr  string
	taskIDExpr string
}

func breakdownGroupSpecFor(driver, groupBy string) breakdownGroupSpec {
	provider := providerCaseExpr()
	switch groupBy {
	case BreakdownGroupTask:
		return breakdownGroupSpec{keyExpr: "e.task_id", labelExpr: "COALESCE(t.title, '')", taskIDExpr: "e.task_id"}
	case BreakdownGroupModel:
		return breakdownGroupSpec{keyExpr: "e.model", labelExpr: "e.model", taskIDExpr: "''"}
	case BreakdownGroupProvider:
		return breakdownGroupSpec{keyExpr: provider, labelExpr: provider, taskIDExpr: "''"}
	case BreakdownGroupAgent:
		return breakdownGroupSpec{keyExpr: "e.agent_type", labelExpr: "e.agent_type", taskIDExpr: "''"}
	case BreakdownGroupDay:
		day := dialect.DateText(driver, "e.occurred_at")
		return breakdownGroupSpec{keyExpr: day, labelExpr: day, taskIDExpr: "''"}
	default:
		return breakdownGroupSpec{
			keyExpr:    "COALESCE(e.session_id, '')",
			labelExpr:  "TRIM(COALESCE(t.title, '') || ' ' || COALESCE(s.name, ''))",
			taskIDExpr: "MAX(e.task_id)",
		}
	}
}

const breakdownFrom = ` FROM task_usage_events e
	LEFT JOIN tasks t ON t.id = e.task_id
	LEFT JOIN task_sessions s ON s.id = e.session_id
	WHERE `

// breakdownFilters builds the shared WHERE body and its arguments. Every
// caller-supplied value is bound as a parameter; only closed-set column names
// and the constant provider expression are interpolated.
func breakdownFilters(q BreakdownQuery, since time.Time) (string, []any) {
	where := "e.occurred_at >= ?"
	args := []any{since}
	if q.Provider != "" {
		where += " AND " + providerCaseExpr() + " = ?"
		args = append(args, q.Provider)
	}
	if q.Model != "" {
		where += " AND e.model = ?"
		args = append(args, q.Model)
	}
	if q.AgentType != "" {
		where += " AND e.agent_type = ?"
		args = append(args, q.AgentType)
	}
	if q.TaskID != "" {
		where += " AND e.task_id = ?"
		args = append(args, q.TaskID)
	}
	if q.SessionID != "" {
		where += " AND e.session_id = ?"
		args = append(args, q.SessionID)
	}
	if q.Q != "" {
		where += " AND LOWER(COALESCE(t.title, '') || ' ' || COALESCE(s.name, '') || ' ' || e.model || ' ' || e.agent_type || ' ' || e.provider) LIKE ?"
		args = append(args, "%"+strings.ToLower(q.Q)+"%")
	}
	return where, args
}

// breakdownSortExpr maps a whitelisted sort key to its aggregate expression.
func breakdownSortExpr(sort string) string {
	switch sort {
	case BreakdownSortCost:
		return "COALESCE(SUM(e.cost_subcents), 0)"
	case BreakdownSortEvents:
		return "COUNT(*)"
	case BreakdownSortLastAt:
		return "MAX(e.occurred_at)"
	default:
		return "COALESCE(SUM(e.tokens_total), 0)"
	}
}

func breakdownRowsQuery(state breakdownQuery) string {
	direction := "DESC"
	if state.Order == BreakdownOrderAsc {
		direction = "ASC"
	}
	return "SELECT " +
		state.spec.keyExpr + " AS row_key, " +
		state.spec.labelExpr + " AS row_label, " +
		state.spec.taskIDExpr + " AS row_task_id, " +
		"COALESCE(MAX(s.name), '') AS row_session_name, " +
		"MAX(" + providerCaseExpr() + ") AS row_provider, " +
		"COALESCE(MAX(e.model), '') AS row_model, " +
		"COUNT(DISTINCT e.model) AS row_model_count, " +
		"COALESCE(SUM(e.tokens_total), 0) AS row_tokens_total, " +
		"COALESCE(SUM(e.tokens_in), 0) AS row_tokens_in, " +
		"COALESCE(SUM(COALESCE(e.tokens_out, 0)), 0) AS row_tokens_out, " +
		"COALESCE(SUM(COALESCE(e.tokens_cached_read, 0)), 0) AS row_tokens_cached_read, " +
		"COALESCE(SUM(e.cost_subcents), 0) AS row_cost_subcents, " +
		"COUNT(*) AS row_events, " +
		"MIN(e.occurred_at) AS row_first_at, " +
		"MAX(e.occurred_at) AS row_last_at" +
		breakdownFrom + state.where +
		" GROUP BY " + state.spec.keyExpr +
		" ORDER BY " + breakdownSortExpr(state.Sort) + " " + direction + ", row_key ASC" +
		" LIMIT ? OFFSET ?"
}

func breakdownTotalsQuery(state breakdownQuery) string {
	return "SELECT COUNT(DISTINCT " + state.spec.keyExpr + "), " +
		"COALESCE(SUM(e.tokens_total), 0), " +
		"COALESCE(SUM(e.cost_subcents), 0), " +
		"COUNT(*)" + breakdownFrom + state.where
}

func breakdownFacetsQuery() string {
	return "SELECT DISTINCT " + providerCaseExpr() + " AS p, e.model AS m, e.agent_type AS a" +
		breakdownFrom + "e.occurred_at >= ?"
}

// breakdownState carries the where body and its bound arguments between the
// query builders and the executors.
type breakdownQuery struct {
	BreakdownQuery
	where string
	args  []any
	spec  breakdownGroupSpec
}

// Breakdown returns the grouped ledger projection for the validated query.
func (s *Service) Breakdown(ctx context.Context, q BreakdownQuery) (*BreakdownResponse, error) {
	if err := q.normalize(); err != nil {
		return nil, err
	}
	canonical, span := parseRange(q.Range)
	q.Range = canonical
	return s.repo.Breakdown(ctx, q, time.Now().UTC().Add(-span))
}

// Breakdown executes the three breakdown queries and assembles the response.
func (r *Repository) Breakdown(ctx context.Context, q BreakdownQuery, since time.Time) (*BreakdownResponse, error) {
	if err := q.normalize(); err != nil {
		return nil, err
	}
	where, args := breakdownFilters(q, since)
	state := breakdownQuery{
		BreakdownQuery: q,
		where:          where,
		args:           args,
		spec:           breakdownGroupSpecFor(r.driver, q.GroupBy),
	}
	rows, err := r.breakdownRows(ctx, state)
	if err != nil {
		return nil, err
	}
	totalRows, totalTokens, totalCost, totalEvents, err := r.breakdownTotals(ctx, state)
	if err != nil {
		return nil, err
	}
	facets, err := r.breakdownFacets(ctx, since)
	if err != nil {
		return nil, err
	}
	return &BreakdownResponse{
		Range:             q.Range,
		GroupBy:           q.GroupBy,
		TotalRows:         totalRows,
		TotalTokens:       totalTokens,
		TotalCostSubcents: totalCost,
		TotalEvents:       totalEvents,
		Facets:            facets,
		Rows:              rows,
	}, nil
}

func (r *Repository) breakdownRows(ctx context.Context, state breakdownQuery) ([]BreakdownRow, error) {
	query := r.reader.Rebind(breakdownRowsQuery(state))
	args := append(append([]any{}, state.args...), state.Limit, state.Offset)
	rows, err := r.reader.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]BreakdownRow, 0)
	for rows.Next() {
		var (
			row        BreakdownRow
			rawFirstAt any
			rawLastAt  any
		)
		if err := rows.Scan(
			&row.Key, &row.Label, &row.TaskID, &row.SessionName, &row.Provider,
			&row.Model, &row.Models, &row.TokensTotal, &row.TokensIn, &row.TokensOut,
			&row.TokensCachedRead, &row.CostSubcents, &row.Events, &rawFirstAt, &rawLastAt,
		); err != nil {
			return nil, err
		}
		firstAt, err := parseBreakdownTimestamp(rawFirstAt)
		if err != nil {
			return nil, err
		}
		lastAt, err := parseBreakdownTimestamp(rawLastAt)
		if err != nil {
			return nil, err
		}
		row.FirstAt = firstAt
		row.LastAt = lastAt
		out = append(out, row)
	}
	return out, rows.Err()
}

// parseBreakdownTimestamp normalizes an aggregate MIN/MAX result. SQLite
// returns aggregate timestamps as text rather than time.Time, so the same
// layouts the task repository uses are accepted here.
func parseBreakdownTimestamp(raw any) (time.Time, error) {
	switch value := raw.(type) {
	case nil:
		return time.Time{}, nil
	case time.Time:
		return value.UTC(), nil
	case string:
		return parseBreakdownTimestampString(value)
	case []byte:
		return parseBreakdownTimestampString(string(value))
	default:
		return time.Time{}, fmt.Errorf("unsupported timestamp type %T", raw)
	}
}

func parseBreakdownTimestampString(value string) (time.Time, error) {
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed.UTC(), nil
	}
	if normalized := strings.Replace(value, " ", "T", 1); normalized != value {
		if parsed, err := time.Parse(time.RFC3339Nano, normalized); err == nil {
			return parsed.UTC(), nil
		}
	}
	for _, layout := range []string{"2006-01-02 15:04:05.999999999", "2006-01-02 15:04:05"} {
		if parsed, err := time.ParseInLocation(layout, value, time.UTC); err == nil {
			return parsed.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid breakdown timestamp %q", value)
}

func (r *Repository) breakdownTotals(ctx context.Context, state breakdownQuery) (int64, int64, int64, int64, error) {
	query := r.reader.Rebind(breakdownTotalsQuery(state))
	var totalRows, totalTokens, totalCost, totalEvents int64
	if err := r.reader.QueryRowContext(ctx, query, state.args...).Scan(
		&totalRows, &totalTokens, &totalCost, &totalEvents); err != nil {
		return 0, 0, 0, 0, err
	}
	return totalRows, totalTokens, totalCost, totalEvents, nil
}

func (r *Repository) breakdownFacets(ctx context.Context, since time.Time) (BreakdownFacets, error) {
	query := r.reader.Rebind(breakdownFacetsQuery())
	rows, err := r.reader.QueryContext(ctx, query, since)
	if err != nil {
		return BreakdownFacets{}, err
	}
	defer func() { _ = rows.Close() }()

	providers := map[string]bool{}
	models := map[string]bool{}
	agents := map[string]bool{}
	for rows.Next() {
		var provider, model, agent string
		if err := rows.Scan(&provider, &model, &agent); err != nil {
			return BreakdownFacets{}, err
		}
		if provider != "" {
			providers[provider] = true
		}
		if model != "" {
			models[model] = true
		}
		if agent != "" {
			agents[agent] = true
		}
	}
	if err := rows.Err(); err != nil {
		return BreakdownFacets{}, err
	}
	return BreakdownFacets{
		Providers:  sortedKeys(providers),
		Models:     sortedKeys(models),
		AgentTypes: sortedKeys(agents),
	}, nil
}

func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for key := range set {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}
