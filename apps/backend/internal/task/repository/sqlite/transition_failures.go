package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// migrateTaskTransitionFailures installs the store for a task's current
// unresolved asynchronous transition failure. One row per task: a newer
// attempt supersedes the previous one.
func (r *Repository) migrateTaskTransitionFailures() error {
	return r.migrate.Apply("task_transition_failures", `
		CREATE TABLE IF NOT EXISTS task_transition_failures (
			task_id TEXT PRIMARY KEY,
			attempt_id TEXT NOT NULL,
			source TEXT NOT NULL,
			session_id TEXT NOT NULL DEFAULT '',
			failure TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL,
			FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
		)`)
}

// SetTaskTransitionFailure stores record as the task's current failure.
func (r *Repository) SetTaskTransitionFailure(ctx context.Context, record models.TaskTransitionFailure) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_transition_failures (task_id, attempt_id, source, session_id, failure, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (task_id) DO UPDATE SET
			attempt_id = excluded.attempt_id, source = excluded.source, session_id = excluded.session_id,
			failure = excluded.failure, created_at = excluded.created_at`),
		record.TaskID, record.AttemptID, record.Source, record.SessionID, record.Failure, record.CreatedAt.UTC())
	return err
}

// GetTaskTransitionFailure returns the task's current failure, or nil.
func (r *Repository) GetTaskTransitionFailure(ctx context.Context, taskID string) (*models.TaskTransitionFailure, error) {
	record := models.TaskTransitionFailure{TaskID: taskID}
	var createdAt time.Time
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT attempt_id, source, session_id, failure, created_at
		FROM task_transition_failures WHERE task_id = ?`), taskID).
		Scan(&record.AttemptID, &record.Source, &record.SessionID, &record.Failure, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	record.CreatedAt = createdAt.UTC()
	return &record, nil
}

// ClearTaskTransitionFailure removes the task's current failure and reports
// whether one existed.
func (r *Repository) ClearTaskTransitionFailure(ctx context.Context, taskID string) (bool, error) {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM task_transition_failures WHERE task_id = ?`), taskID)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected > 0, err
}
