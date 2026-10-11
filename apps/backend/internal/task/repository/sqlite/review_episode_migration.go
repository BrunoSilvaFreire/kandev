package sqlite

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/api/v1"
)

// migrateTaskRecoveryReviewEpisodes installs the additive recovery/Review
// episode columns and backfills conservative legacy baselines. It is
// idempotent: re-running on the same database is a no-op (guarded updates)
// and it never rewrites a genuine episode recorded after deployment.
// See REQ-TASKS-RECOVERY-REVIEW-001.
func (r *Repository) migrateTaskRecoveryReviewEpisodes() error {
	_ = r.migrate.Apply("task_sessions.interruption_episode_id", `ALTER TABLE task_sessions ADD COLUMN interruption_episode_id TEXT NOT NULL DEFAULT ''`)
	_ = r.migrate.Apply("task_sessions.interruption_reason", `ALTER TABLE task_sessions ADD COLUMN interruption_reason TEXT NOT NULL DEFAULT ''`)
	_ = r.migrate.Apply("task_sessions.interruption_observed_at", `ALTER TABLE task_sessions ADD COLUMN interruption_observed_at TIMESTAMP`)
	_ = r.migrate.Apply("task_sessions.interruption_turn_id", `ALTER TABLE task_sessions ADD COLUMN interruption_turn_id TEXT NOT NULL DEFAULT ''`)
	_ = r.migrate.Apply("tasks.review_episode_id", `ALTER TABLE tasks ADD COLUMN review_episode_id TEXT NOT NULL DEFAULT ''`)
	_ = r.migrate.Apply("tasks.review_entered_at", `ALTER TABLE tasks ADD COLUMN review_entered_at TIMESTAMP`)
	_ = r.migrate.Apply("tasks.review_entry_origin", `ALTER TABLE tasks ADD COLUMN review_entry_origin TEXT NOT NULL DEFAULT ''`)

	ctx := context.Background()
	if err := r.backfillObservedReviewBaselines(ctx); err != nil {
		return err
	}
	return r.backfillLegacyInterruptions(ctx)
}

// backfillObservedReviewBaselines labels tasks already in Review with a
// conservative observed-at-migration entry time. It never overwrites an
// existing episode, so replay after deployment cannot rewrite genuine data.
func (r *Repository) backfillObservedReviewBaselines(ctx context.Context) error {
	rows, err := r.db.QueryxContext(ctx, r.db.Rebind(`
		SELECT id FROM tasks
		WHERE state = ? AND (review_episode_id IS NULL OR review_episode_id = '')
	`), string(v1.TaskStateReview))
	if err != nil {
		return err
	}
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	_ = rows.Close()
	now := r.nowUTC()
	for _, id := range ids {
		if _, err := r.db.ExecContext(ctx, r.db.Rebind(`
			UPDATE tasks SET review_episode_id = ?, review_entered_at = ?, review_entry_origin = ?
			WHERE id = ? AND (review_episode_id IS NULL OR review_episode_id = '')
		`), uuid.NewString(), now, models.ReviewEntryOriginObservedBaseline, id); err != nil {
			return err
		}
	}
	return nil
}

// backfillLegacyInterruptions creates an unknown_legacy session episode only
// when a task carries the legacy interrupted_at marker and exactly one
// non-terminal session is provable. Ambiguous tasks keep only the marker.
func (r *Repository) backfillLegacyInterruptions(ctx context.Context) error {
	rows, err := r.db.QueryxContext(ctx, `SELECT id, metadata FROM tasks`)
	if err != nil {
		return err
	}
	candidates := make([]string, 0)
	for rows.Next() {
		var id string
		var metadata []byte
		if err := rows.Scan(&id, &metadata); err != nil {
			_ = rows.Close()
			return err
		}
		if metadataHasInterruptedAt(metadata) {
			candidates = append(candidates, id)
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	_ = rows.Close()
	now := r.nowUTC()
	for _, taskID := range candidates {
		sessionIDs, err := r.legacyInterruptedSessionCandidates(ctx, taskID)
		if err != nil {
			return err
		}
		if len(sessionIDs) != 1 {
			continue
		}
		if _, err := r.db.ExecContext(ctx, r.db.Rebind(`
			UPDATE task_sessions
			SET interruption_episode_id = ?, interruption_reason = ?, interruption_observed_at = ?
			WHERE id = ? AND (interruption_episode_id IS NULL OR interruption_episode_id = '')
		`), uuid.NewString(), models.InterruptionReasonUnknownLegacy, now, sessionIDs[0]); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) legacyInterruptedSessionCandidates(ctx context.Context, taskID string) ([]string, error) {
	rows, err := r.db.QueryxContext(ctx, r.db.Rebind(`
		SELECT id FROM task_sessions
		WHERE task_id = ? AND state NOT IN ('COMPLETED', 'CANCELLED', 'FAILED')
		ORDER BY started_at, id
	`), taskID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	ids := make([]string, 0, 2)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func metadataHasInterruptedAt(metadata []byte) bool {
	if len(metadata) == 0 {
		return false
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(metadata, &parsed); err != nil {
		return false
	}
	_, ok := parsed[models.MetaKeyInterruptedAt]
	return ok
}
