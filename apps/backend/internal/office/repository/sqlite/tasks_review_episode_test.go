package sqlite_test

import (
	"context"
	"database/sql"
	"testing"
)

// A state write keeps the task's Review episode in step: entering Review
// allocates one, staying preserves it, and leaving clears it.
func TestUpdateTaskStateMaintainsReviewEpisode(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()
	insertTask(t, repo, ctx, "rv-1", "ws-1", "Title", "", "KAN-9")

	episode := func() (string, bool) {
		t.Helper()
		var id string
		var entered sql.NullTime
		if err := repo.ReaderDB().QueryRowContext(ctx,
			`SELECT review_episode_id, review_entered_at FROM tasks WHERE id = 'rv-1'`,
		).Scan(&id, &entered); err != nil {
			t.Fatalf("read episode: %v", err)
		}
		return id, entered.Valid
	}
	set := func(state string) {
		t.Helper()
		if err := repo.UpdateTaskState(ctx, "rv-1", state); err != nil {
			t.Fatalf("UpdateTaskState(%s): %v", state, err)
		}
	}

	set("REVIEW")
	first, entered := episode()
	if first == "" || !entered {
		t.Fatalf("entering Review must allocate an episode, got id=%q entered=%v", first, entered)
	}
	set("REVIEW")
	if again, _ := episode(); again != first {
		t.Fatalf("staying in Review changed the episode: %q -> %q", first, again)
	}
	set("IN_PROGRESS")
	if id, stillEntered := episode(); id != "" || stillEntered {
		t.Fatalf("leaving Review must clear the episode, got id=%q entered=%v", id, stillEntered)
	}
	set("REVIEW")
	if reentered, _ := episode(); reentered == "" || reentered == first {
		t.Fatalf("re-entry must allocate a new episode, got %q (first %q)", reentered, first)
	}
}
