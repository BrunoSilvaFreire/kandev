package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/entryroute"
)

func TestUpdateTaskMovePersistsPendingEntryRouteAtomically(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	ctx := context.Background()
	createStepTransitionsTestTask(t, repo, "task-route", "wf-1", "step-a")

	rows := stepTransitionRowsForTask(t, repo, "task-route")
	if len(rows) != 1 {
		t.Fatalf("genesis rows = %d, want 1", len(rows))
	}

	moveCtx := entryroute.WithPendingRoute(ctx, entryroute.PendingRoute{
		DestinationStepID: "step-b",
		AgentProfileID:    "profile-review",
		StartPolicy:       "new",
		SourceSessionID:   "session-1",
	})
	task, err := repo.GetTask(ctx, "task-route")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	task.WorkflowStepID = "step-b"
	if err := repo.UpdateTask(moveCtx, task); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}

	moved := stepTransitionRowsForTask(t, repo, "task-route")
	if len(moved) != 2 {
		t.Fatalf("rows = %d, want 2", len(moved))
	}
	moveID := moved[1].id

	reread, err := repo.GetTask(ctx, "task-route")
	if err != nil {
		t.Fatalf("re-read task: %v", err)
	}
	route, ok := models.LoadWorkflowSessionRoute(reread.Metadata)
	if !ok {
		t.Fatal("workflow_session_route was not persisted")
	}
	if route.AgentProfileID != "profile-review" {
		t.Fatalf("agent_profile_id = %q, want profile-review", route.AgentProfileID)
	}
	if route.DestinationStepID != "step-b" {
		t.Fatalf("destination_step_id = %q, want step-b", route.DestinationStepID)
	}
	if route.EntryIdentity != entryroute.EntryIdentity(moveID) {
		t.Fatalf("entry_identity = %q, want %q", route.EntryIdentity, entryroute.EntryIdentity(moveID))
	}
	if route.TargetKind != entryroute.TargetKindProfile {
		t.Fatalf("target_kind = %q, want profile", route.TargetKind)
	}
	if route.SourceSessionID != "session-1" {
		t.Fatalf("source_session_id = %q, want session-1", route.SourceSessionID)
	}
}

func TestUpdateTaskMoveDiscardsDivertedPendingEntryRoute(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	ctx := context.Background()
	createStepTransitionsTestTask(t, repo, "task-diverted", "wf-1", "step-a")

	moveCtx := entryroute.WithPendingRoute(ctx, entryroute.PendingRoute{
		DestinationStepID: "step-other",
		AgentProfileID:    "profile-review",
		StartPolicy:       "new",
	})
	task, err := repo.GetTask(ctx, "task-diverted")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	task.WorkflowStepID = "step-b"
	if err := repo.UpdateTask(moveCtx, task); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}

	reread, err := repo.GetTask(ctx, "task-diverted")
	if err != nil {
		t.Fatalf("re-read task: %v", err)
	}
	if _, ok := models.LoadWorkflowSessionRoute(reread.Metadata); ok {
		t.Fatal("diverted pending route must not be persisted")
	}
}
