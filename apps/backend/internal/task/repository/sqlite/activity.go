package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// Activity projection defaults.
const (
	defaultActivityLimit = 50
	maxActivityLimit     = 100
)

func normalizeActivityLimit(limit int) int {
	if limit <= 0 {
		return defaultActivityLimit
	}
	if limit > maxActivityLimit {
		return maxActivityLimit
	}
	return limit
}

// ListTaskActivityTransitions returns committed step transitions for a task,
// newest first by (occurred_at, id), optionally narrowed to a destination step
// and/or bounded by an exclusive cursor timestamp.
func (r *Repository) ListTaskActivityTransitions(
	ctx context.Context,
	taskID string,
	filter models.TaskActivityFilter,
) ([]*models.TaskStepTransition, error) {
	limit := normalizeActivityLimit(filter.Limit)
	query := `SELECT id, task_id, session_id, from_workflow_id, from_workflow_step_id,
			to_workflow_id, to_workflow_step_id, trigger, actor_kind, actor_id,
			trigger_detail, contract_version, occurred_at
		FROM task_step_transitions WHERE task_id = ?`
	args := []interface{}{taskID}
	if filter.StepID != "" {
		query += " AND to_workflow_step_id = ?"
		args = append(args, filter.StepID)
	}
	if filter.Before != nil {
		query += " AND occurred_at < ?"
		args = append(args, *filter.Before)
	}
	query += " ORDER BY occurred_at DESC, id DESC LIMIT ?"
	args = append(args, limit+1)
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query), args...)
	if err != nil {
		return nil, fmt.Errorf("list task activity transitions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	result := make([]*models.TaskStepTransition, 0, limit+1)
	for rows.Next() {
		row := &models.TaskStepTransition{}
		var sessionID, fromWF, fromStep, toWF, toStep, actorID, detail sql.NullString
		if err := rows.Scan(
			&row.ID, &row.TaskID, &sessionID, &fromWF, &fromStep, &toWF, &toStep,
			&row.Trigger, &row.ActorKind, &actorID, &detail, &row.ContractVersion, &row.OccurredAt,
		); err != nil {
			return nil, err
		}
		row.SessionID = nullStringPtr(sessionID)
		row.FromWorkflowID = fromWF.String
		row.FromWorkflowStepID = fromStep.String
		row.ToWorkflowID = toWF.String
		row.ToWorkflowStepID = toStep.String
		row.ActorID = actorID.String
		row.TriggerDetail = detail.String
		result = append(result, row)
	}
	return result, rows.Err()
}

// CountCommittedTransitionPairs aggregates successfully committed transitions
// by directed (from_step_id, to_step_id) for a task. Rows with an empty source
// (the genesis/initial entry) are excluded, since they are not a configured
// edge. Blocked/evaluated attempts never write ledger rows, so every counted
// row is a committed invocation.
func (r *Repository) CountCommittedTransitionPairs(
	ctx context.Context,
	taskID string,
) ([]models.StepTransitionCount, error) {
	query := `SELECT from_workflow_step_id, to_workflow_step_id, COUNT(*)
		FROM task_step_transitions
		WHERE task_id = ? AND from_workflow_step_id IS NOT NULL AND from_workflow_step_id <> ''
			AND to_workflow_step_id IS NOT NULL AND to_workflow_step_id <> ''
		GROUP BY from_workflow_step_id, to_workflow_step_id`
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query), taskID)
	if err != nil {
		return nil, fmt.Errorf("count committed transition pairs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var result []models.StepTransitionCount
	for rows.Next() {
		var pair models.StepTransitionCount
		if err := rows.Scan(&pair.FromStepID, &pair.ToStepID, &pair.Count); err != nil {
			return nil, err
		}
		result = append(result, pair)
	}
	return result, rows.Err()
}

// ListTaskActivityDocumentRevisions returns document revision metadata for a
// task (no bodies), newest first, optionally bounded by an exclusive cursor.
func (r *Repository) ListTaskActivityDocumentRevisions(
	ctx context.Context,
	taskID string,
	before *time.Time,
	limit int,
) ([]*models.TaskDocumentRevision, error) {
	limit = normalizeActivityLimit(limit)
	query := `SELECT id, task_id, document_key, revision_number, title, author_kind,
			author_name, source_task_id, source_session_id, source_workflow_step_id,
			created_at, updated_at
		FROM task_document_revisions WHERE task_id = ?`
	args := []interface{}{taskID}
	if before != nil {
		query += " AND created_at < ?"
		args = append(args, *before)
	}
	query += " ORDER BY created_at DESC, id DESC LIMIT ?"
	args = append(args, limit+1)
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query), args...)
	if err != nil {
		return nil, fmt.Errorf("list task activity document revisions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	result := make([]*models.TaskDocumentRevision, 0, limit+1)
	for rows.Next() {
		rev := &models.TaskDocumentRevision{}
		var sourceTask, sourceSession, sourceStep sql.NullString
		if err := rows.Scan(
			&rev.ID, &rev.TaskID, &rev.DocumentKey, &rev.RevisionNumber, &rev.Title,
			&rev.AuthorKind, &rev.AuthorName, &sourceTask, &sourceSession, &sourceStep,
			&rev.CreatedAt, &rev.UpdatedAt,
		); err != nil {
			return nil, err
		}
		rev.SourceTaskID = nullStringPtr(sourceTask)
		rev.SourceSessionID = nullStringPtr(sourceSession)
		rev.SourceWorkflowStepID = nullStringPtr(sourceStep)
		result = append(result, rev)
	}
	return result, rows.Err()
}

// CountStepEntriesByTask returns, per destination step, the number of committed
// entries for a task. The genesis row (empty source) is included: it is a real
// visit to the initial step even though it is not a configured edge.
func (r *Repository) CountStepEntriesByTask(
	ctx context.Context,
	taskID string,
) ([]models.StepVisitSummary, error) {
	query := `SELECT to_workflow_step_id, COUNT(*)
		FROM task_step_transitions
		WHERE task_id = ? AND to_workflow_step_id IS NOT NULL AND to_workflow_step_id <> ''
		GROUP BY to_workflow_step_id`
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query), taskID)
	if err != nil {
		return nil, fmt.Errorf("count step entries by task: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var result []models.StepVisitSummary
	for rows.Next() {
		var visit models.StepVisitSummary
		if err := rows.Scan(&visit.StepID, &visit.Count); err != nil {
			return nil, err
		}
		result = append(result, visit)
	}
	return result, rows.Err()
}

// ListTaskActivityReviewRuns returns a task's review runs newest-first,
// optionally bounded by an exclusive cursor timestamp. Bounding in SQL is
// required for cursor pagination: fetching the newest `limit` rows and
// filtering the page in memory would silently skip older runs.
func (r *Repository) ListTaskActivityReviewRuns(
	ctx context.Context,
	taskID string,
	stepID string,
	before *time.Time,
	limit int,
) ([]*models.TaskReviewRun, error) {
	limit = normalizeActivityLimit(limit)
	query := `SELECT ` + reviewRunColumns + ` FROM task_review_runs WHERE task_id = ?`
	args := []interface{}{taskID}
	if stepID != "" {
		query += " AND workflow_step_id = ?"
		args = append(args, stepID)
	}
	if before != nil {
		query += " AND created_at < ?"
		args = append(args, *before)
	}
	query += " ORDER BY created_at DESC, id DESC LIMIT ?"
	args = append(args, limit+1)
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query), args...)
	if err != nil {
		return nil, fmt.Errorf("list task activity review runs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return collectReviewRuns(rows)
}
