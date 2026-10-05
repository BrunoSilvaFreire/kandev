package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/task/models"
)

// sessionRouteSelectCols is the fixed column order shared by every SELECT that
// scans into a *models.TaskSessionRoute.
const sessionRouteSelectCols = `id, task_id, destination_workflow_step_id,
	source_session_id, destination_session_id, agent_profile_id,
	start_policy, end_policy, outcome, reason, decision_detail,
	workflow_step_transition_id, correlation_id, created_at`

// RecordSessionRoute appends one durable routing decision to the task
// session-route ledger. A non-empty correlation_id makes the write idempotent:
// replaying the same decision for a task inserts nothing. The unique partial
// index is the durable backstop, so no read-then-write race can duplicate a
// row.
func (r *Repository) RecordSessionRoute(ctx context.Context, route *models.TaskSessionRoute) error {
	if route == nil {
		return fmt.Errorf("record session route: nil route")
	}
	if route.ID == "" {
		route.ID = uuid.New().String()
	}
	if route.CreatedAt.IsZero() {
		route.CreatedAt = time.Now().UTC()
	}

	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_session_routes
			(`+sessionRouteSelectCols+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT DO NOTHING
	`),
		route.ID, route.TaskID, route.DestinationWorkflowStepID,
		nullableStringPtr(route.SourceSessionID), nullableStringPtr(route.DestinationSessionID),
		route.AgentProfileID, route.StartPolicy, route.EndPolicy,
		string(route.Outcome), string(route.Reason), nullableStringPtr(route.DecisionDetail),
		nullableInt64Ptr(route.WorkflowStepTransitionID), route.CorrelationID, route.CreatedAt)
	if err != nil {
		return fmt.Errorf("record session route: %w", err)
	}
	return nil
}

// ListTaskSessionRoutes returns the task's durable routing decisions in
// chronological order with a stable id tie-break. Deleted sessions leave their
// rows intact with nil session links.
func (r *Repository) ListTaskSessionRoutes(ctx context.Context, taskID string) ([]*models.TaskSessionRoute, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT `+sessionRouteSelectCols+`
		FROM task_session_routes
		WHERE task_id = ?
		ORDER BY created_at, id
	`), taskID)
	if err != nil {
		return nil, fmt.Errorf("list task session routes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*models.TaskSessionRoute
	for rows.Next() {
		route := &models.TaskSessionRoute{}
		var sourceSession, destinationSession, decisionDetail sql.NullString
		var transitionID sql.NullInt64
		if err := rows.Scan(
			&route.ID, &route.TaskID, &route.DestinationWorkflowStepID,
			&sourceSession, &destinationSession, &route.AgentProfileID,
			&route.StartPolicy, &route.EndPolicy, &route.Outcome, &route.Reason, &decisionDetail,
			&transitionID, &route.CorrelationID, &route.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan task session route: %w", err)
		}
		route.SourceSessionID = nullStringPtr(sourceSession)
		route.DestinationSessionID = nullStringPtr(destinationSession)
		route.DecisionDetail = nullStringPtr(decisionDetail)
		if transitionID.Valid {
			value := transitionID.Int64
			route.WorkflowStepTransitionID = &value
		}
		out = append(out, route)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate task session routes: %w", err)
	}
	return out, nil
}

// nullableStringPtr renders a nullable string pointer as SQL NULL when nil.
func nullableStringPtr(value *string) interface{} {
	if value == nil || *value == "" {
		return nil
	}
	return *value
}

// nullableInt64Ptr renders a nullable int64 pointer as SQL NULL when nil.
func nullableInt64Ptr(value *int64) interface{} {
	if value == nil {
		return nil
	}
	return *value
}
