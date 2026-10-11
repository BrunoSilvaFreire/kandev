package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestTaskTransitionFailureSupersedesAndClears(t *testing.T) {
	repo := newRepoForCompletionGateTests(t)
	ctx := context.Background()
	task := createTestTaskForGate(t, ctx, repo, "task-transition-failure")

	if got, err := repo.GetTaskTransitionFailure(ctx, task.ID); err != nil || got != nil {
		t.Fatalf("empty read = %+v, %v", got, err)
	}
	first := models.TaskTransitionFailure{
		TaskID: task.ID, AttemptID: "move-1", Source: models.TransitionFailureSourceDeferredMove,
		SessionID: "s1", Failure: `{"code":"a"}`, CreatedAt: time.Now().UTC().Truncate(time.Second),
	}
	if err := repo.SetTaskTransitionFailure(ctx, first); err != nil {
		t.Fatalf("set first: %v", err)
	}
	second := first
	second.AttemptID, second.Source, second.Failure = "advance-2", models.TransitionFailureSourceAutoAdvance, `{"code":"b"}`
	if err := repo.SetTaskTransitionFailure(ctx, second); err != nil {
		t.Fatalf("set second: %v", err)
	}
	got, err := repo.GetTaskTransitionFailure(ctx, task.ID)
	if err != nil || got == nil {
		t.Fatalf("read = %+v, %v", got, err)
	}
	if got.AttemptID != "advance-2" || got.Source != models.TransitionFailureSourceAutoAdvance || got.Failure != `{"code":"b"}` {
		t.Fatalf("newer attempt did not supersede: %+v", got)
	}

	// Replaying the schema step keeps the row.
	if err := repo.migrateTaskTransitionFailures(); err != nil {
		t.Fatalf("replay migration: %v", err)
	}
	if got, _ := repo.GetTaskTransitionFailure(ctx, task.ID); got == nil {
		t.Fatal("replay dropped the failure")
	}

	cleared, err := repo.ClearTaskTransitionFailure(ctx, task.ID)
	if err != nil || !cleared {
		t.Fatalf("clear = %v, %v", cleared, err)
	}
	if again, _ := repo.ClearTaskTransitionFailure(ctx, task.ID); again {
		t.Fatal("second clear reported a row")
	}
}
