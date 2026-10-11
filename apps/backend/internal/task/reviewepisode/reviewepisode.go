// Package reviewepisode keeps a task's active Review episode consistent with
// a task-state write. Every statement that writes tasks.state either runs the
// task repository's transactional helper or embeds SetClause, so no writer
// can enter or leave Review without the episode following it.
// See REQ-TASKS-RECOVERY-REVIEW-001.
package reviewepisode

import (
	"time"

	"github.com/google/uuid"
)

const (
	stateReview = "REVIEW"
	// OriginUnknown is recorded by writers that do not know why the task
	// entered Review; it never claims a cause.
	OriginUnknown = "unknown_legacy"
)

// SetClause returns a SET fragment (no leading or trailing comma) and its
// arguments for an UPDATE of tasks that also writes state = nextState. It
// reads the row's pre-update state: entering Review allocates an episode,
// staying in Review preserves it, and any other state clears it.
func SetClause(nextState, origin string, now time.Time) (string, []interface{}) {
	if origin == "" {
		origin = OriginUnknown
	}
	const clause = `review_episode_id = CASE WHEN ? = '` + stateReview + `' THEN
			(CASE WHEN state = '` + stateReview + `' AND COALESCE(review_episode_id, '') != '' THEN review_episode_id ELSE ? END)
			ELSE '' END,
		review_entered_at = CASE WHEN ? = '` + stateReview + `' THEN
			(CASE WHEN state = '` + stateReview + `' AND review_entered_at IS NOT NULL THEN review_entered_at ELSE ? END)
			ELSE NULL END,
		review_entry_origin = CASE WHEN ? = '` + stateReview + `' THEN
			(CASE WHEN state = '` + stateReview + `' AND COALESCE(review_entry_origin, '') != '' THEN review_entry_origin ELSE ? END)
			ELSE '' END`
	return clause, []interface{}{nextState, uuid.NewString(), nextState, now, nextState, origin}
}
