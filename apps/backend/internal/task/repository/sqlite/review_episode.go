package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/reviewepisode"
	"github.com/kandev/kandev/pkg/api/v1"
)

// MarkSessionInterrupted records one typed interruption episode for a session
// whose active execution was lost. It is idempotent: a repeated sweep preserves
// the first episode identity and observed time because the guarded update only
// matches a session with no episode yet. See REQ-TASKS-RECOVERY-REVIEW-001.
func (r *Repository) MarkSessionInterrupted(
	ctx context.Context, sessionID, reason, turnID string, observedAt time.Time,
) error {
	if strings.TrimSpace(sessionID) == "" {
		return models.ErrTaskSessionNotFound
	}
	if strings.TrimSpace(reason) == "" {
		reason = models.InterruptionReasonUnknownLegacy
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_sessions
		SET interruption_episode_id = ?, interruption_reason = ?, interruption_observed_at = ?, interruption_turn_id = ?
		WHERE id = ? AND (interruption_episode_id IS NULL OR interruption_episode_id = '')
	`), uuid.NewString(), reason, observedAt, strings.TrimSpace(turnID), sessionID)
	return err
}

// ResolveSessionInterruption clears one session's interruption episode only
// when the expected episode ID still owns it, so a stale callback cannot clear
// a newer episode.
func (r *Repository) ResolveSessionInterruption(ctx context.Context, sessionID, episodeID string) (bool, error) {
	if strings.TrimSpace(sessionID) == "" || strings.TrimSpace(episodeID) == "" {
		return false, nil
	}
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_sessions
		SET interruption_episode_id = '', interruption_reason = '', interruption_observed_at = NULL, interruption_turn_id = ''
		WHERE id = ? AND interruption_episode_id = ?
	`), sessionID, episodeID)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows == 1, nil
}

// applyTaskReviewEpisodeTx keeps the active Review episode consistent with a
// task-state transition inside the caller's own transaction. A real non-Review
// to Review transition allocates an episode; remaining in Review preserves it;
// leaving Review clears it. Callers that do not know a specific origin record
// unknown_legacy deliberately rather than inventing a cause.
// See REQ-TASKS-RECOVERY-REVIEW-001.
func (r *Repository) applyTaskReviewEpisodeTx(
	ctx context.Context,
	tx *sql.Tx,
	taskID string,
	currentState, nextState v1.TaskState,
	origin string,
	now time.Time,
) error {
	switch {
	case nextState == v1.TaskStateReview && currentState != v1.TaskStateReview:
		if strings.TrimSpace(origin) == "" {
			origin = models.ReviewEntryOriginUnknownLegacy
		}
		_, err := tx.ExecContext(ctx, r.db.Rebind(`
			UPDATE tasks SET review_episode_id = ?, review_entered_at = ?, review_entry_origin = ?
			WHERE id = ?
		`), uuid.NewString(), now, origin, taskID)
		return err
	case nextState != v1.TaskStateReview && currentState == v1.TaskStateReview:
		_, err := tx.ExecContext(ctx, r.db.Rebind(`
			UPDATE tasks SET review_episode_id = '', review_entered_at = NULL, review_entry_origin = ''
			WHERE id = ?
		`), taskID)
		return err
	default:
		return nil
	}
}

// GetSessionInterruptionEpisode returns the session's unresolved episode ID,
// or "" when it carries none.
func (r *Repository) GetSessionInterruptionEpisode(ctx context.Context, sessionID string) (string, error) {
	var episodeID string
	err := r.db.QueryRowContext(ctx, r.db.Rebind(
		`SELECT COALESCE(interruption_episode_id, '') FROM task_sessions WHERE id = ?`,
	), sessionID).Scan(&episodeID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return episodeID, err
}

// syncTaskReviewEpisodeTx reconciles the Review episode with the state a
// writer just stored in the same transaction. Episode fields are only set
// while a task is in Review, so the stored state alone decides whether to
// allocate, preserve, or clear. Writers that do not read the prior state use
// this instead of applyTaskReviewEpisodeTx.
func (r *Repository) syncTaskReviewEpisodeTx(
	ctx context.Context, tx *sql.Tx, taskID string, state v1.TaskState, now time.Time,
) error {
	clause, args := reviewepisode.SetClause(string(state), "", now)
	_, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE tasks SET `+clause+` WHERE id = ?`), append(args, taskID)...)
	return err
}

// CountUnresolvedSessionInterruptions reports how many sessions of a task
// still carry an interruption episode. The task warning marker may be removed
// only when this is zero.
func (r *Repository) CountUnresolvedSessionInterruptions(ctx context.Context, taskID string) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, r.db.Rebind(`
		SELECT COUNT(*) FROM task_sessions
		WHERE task_id = ? AND COALESCE(interruption_episode_id, '') != ''
	`), taskID).Scan(&count)
	return count, err
}

// parseTaskInterruption decodes the packed "episode|reason|session" projection
// selected by taskScanColumns.
func parseTaskInterruption(packed string) *models.TaskInterruption {
	parts := strings.Split(packed, "|")
	if len(parts) != 3 || parts[0] == "" {
		return nil
	}
	return &models.TaskInterruption{EpisodeID: parts[0], Reason: parts[1], SessionID: parts[2]}
}
