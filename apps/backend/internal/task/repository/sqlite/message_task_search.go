package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

// Task-wide search limits. The handler clamps user input to the same bounds;
// the repository clamps again so a direct caller cannot force an unbounded scan.
const (
	defaultTaskSearchLimit = 50
	maxTaskSearchLimit     = 100
)

// SearchTaskMessages returns one keyset page of task messages matching the
// query, in the global order (active-session bucket ASC, created_at DESC,
// id DESC). WorkflowStepID is read only from the owning turn's immutable
// workflow_step_id_at_start stamp; legacy/unstamped turns leave it empty.
// Returns whether more rows remain after this page.
func (r *Repository) SearchTaskMessages(
	ctx context.Context,
	taskID string,
	opts models.SearchTaskMessagesOptions,
) ([]*models.TaskMessageSearchHit, bool, error) {
	query := strings.TrimSpace(opts.Query)
	if query == "" || taskID == "" {
		return nil, false, nil
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = defaultTaskSearchLimit
	}
	if limit > maxTaskSearchLimit {
		limit = maxTaskSearchLimit
	}
	escaper := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	pattern := "%" + escaper.Replace(query) + "%"

	sqlText, args := buildTaskSearchQuery(r.ro.DriverName(), taskID, opts, pattern, limit)
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(sqlText), args...)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = rows.Close() }()
	return scanTaskSearchRows(rows, limit)
}

// buildTaskSearchQuery assembles the task-search keyset query and bound
// arguments. Argument order must follow the placeholders textually.
func buildTaskSearchQuery(
	driverName, taskID string,
	opts models.SearchTaskMessagesOptions,
	pattern string,
	limit int,
) (string, []interface{}) {
	nm := dialect.NormalizedMicrosecond(driverName, "m.created_at")
	bound := "?"
	if dialect.IsPostgres(driverName) {
		bound = conversationTimestampParameter
	}
	like := dialect.Like(driverName)
	stepExpr := dialect.JSONExtract(driverName, "t.metadata", models.TurnMetaKeyWorkflowStepIDAtStart)
	// SQLite renders a missing JSON key as NULL from json_extract and as SQL
	// NULL from the Postgres ->> operator, so COALESCE yields the empty string.
	bucketExpr := "CASE WHEN m.task_session_id = ? THEN 0 ELSE 1 END"

	var b strings.Builder
	fmt.Fprintf(&b, `
		SELECT m.id, m.task_session_id, m.task_id, m.turn_id, m.author_type, m.author_id, m.content,
		       m.requests_input, m.type, m.metadata, m.created_at, m.updated_at,
		       CAST(%s AS TEXT) AS order_key,
		       COALESCE(%s, '') AS step_id
		FROM task_session_messages m
		LEFT JOIN task_session_turns t ON t.id = m.turn_id
		WHERE m.task_id = ? AND m.content %s ? ESCAPE '\'`, nm, stepExpr, like)
	args := []interface{}{taskID, pattern}

	if opts.Cursor != nil {
		fmt.Fprintf(&b, `
			AND (%s > ? OR (%s = ? AND (%s < %s OR (%s = %s AND m.id < ?))))`,
			bucketExpr, bucketExpr, nm, bound, nm, bound)
		args = append(args,
			opts.ActiveSessionID, opts.Cursor.Bucket,
			opts.ActiveSessionID, opts.Cursor.Bucket,
			opts.Cursor.Key, opts.Cursor.Key, opts.Cursor.ID,
		)
	}

	fmt.Fprintf(&b, " ORDER BY %s ASC, %s DESC, m.id DESC", bucketExpr, nm)
	args = append(args, opts.ActiveSessionID)
	b.WriteString(sqlLimitClause)
	args = append(args, limit+1)

	return b.String(), args
}

// scanTaskSearchRows scans the 14-column task-search projection and returns
// the page-truncated slice plus whether more rows remain.
func scanTaskSearchRows(rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}, limit int) ([]*models.TaskMessageSearchHit, bool, error) {
	result := make([]*models.TaskMessageSearchHit, 0, limit)
	for rows.Next() {
		message := &models.Message{}
		hit := &models.TaskMessageSearchHit{Message: message}
		var requestsInput int
		var messageType string
		var metadataJSON string
		var stepID sql.NullString
		if err := rows.Scan(
			&message.ID, &message.TaskSessionID, &message.TaskID, &message.TurnID,
			&message.AuthorType, &message.AuthorID, &message.Content, &requestsInput,
			&messageType, &metadataJSON, &message.CreatedAt, &message.UpdatedAt,
			&hit.OrderKey, &stepID,
		); err != nil {
			return nil, false, err
		}
		message.RequestsInput = requestsInput == 1
		message.Type = models.MessageType(messageType)
		if metadataJSON != "" && metadataJSON != "{}" {
			if err := json.Unmarshal([]byte(metadataJSON), &message.Metadata); err != nil {
				return nil, false, fmt.Errorf("failed to deserialize message metadata: %w", err)
			}
		}
		if stepID.Valid {
			hit.WorkflowStepID = stepID.String
		}
		result = append(result, hit)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	if limit > 0 && len(result) > limit {
		return result[:limit], true, nil
	}
	return result, false, nil
}
