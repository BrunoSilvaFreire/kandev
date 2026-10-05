package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/entryroute"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// TestPromoteSameStepQueuedTaskKeepsFrozenRoute proves same-step WIP admission
// never re-ranks a committed entry: even with a selector that would fail, the
// promotion succeeds with zero selection calls and the route frozen at genesis
// is unchanged (AC-AGENTS-TAGGED-QUOTA-SELECTION-001.11).
func TestPromoteSameStepQueuedTaskKeepsFrozenRoute(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	step := &wfmodels.WorkflowStep{
		ID: "step-s", WorkflowID: "wf-s", Name: "Doing", Position: 1,
		WIPLimit: 1, AllowedTags: []string{"review"},
	}
	svc.SetWorkflowStepGetter(&fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"step-s": step,
	}})
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-s", Name: "S"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-s", WorkspaceID: "ws-s", Name: "S flow"}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	frozen := models.WorkflowSessionRoute{
		OperationID:       "workflow-session:task-same:step-s:entry:00000000000000000001:profile::reuse",
		DestinationStepID: "step-s",
		EntryIdentity:     "entry:00000000000000000001",
		TargetKind:        entryroute.TargetKindProfile,
		AgentProfileID:    "profile-frozen",
		Phase:             "committed",
	}
	candidate := &models.Task{
		ID: "task-same", WorkspaceID: "ws-s", WorkflowID: "wf-s", WorkflowStepID: "step-s",
		Title: "Queued", State: v1.TaskStateTODO, Priority: priorityMedium,
		QueuedForStepID: "step-s",
		Metadata:        map[string]interface{}{models.MetaKeyWorkflowSessionRoute: frozen},
	}
	if err := repo.CreateTask(ctx, candidate); err != nil {
		t.Fatalf("create task: %v", err)
	}

	selector := &recordingEntrySelector{err: errors.New("quota telemetry changed")}
	svc.SetWorkflowEntryProfileSelector(selector)

	if !svc.promoteSameStepQueuedTask(ctx, candidate, "step-s", step, 0, map[string]struct{}{}) {
		t.Fatal("same-step promotion = false, want true")
	}
	if selector.calls != 0 {
		t.Fatalf("selector called %d times, want 0 for same-step promotion", selector.calls)
	}
	stored, err := repo.GetTask(ctx, "task-same")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if !stored.WIPAdmitted {
		t.Fatal("same-step promotion did not admit the task")
	}
	got, ok := models.LoadWorkflowSessionRoute(stored.Metadata)
	if !ok || got != frozen {
		t.Fatalf("frozen route = %#v (ok=%v), want unchanged %#v", got, ok, frozen)
	}
}
