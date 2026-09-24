package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/entryroute"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

type recordingEntrySelector struct {
	calls   int
	profile string
	err     error
}

func (r *recordingEntrySelector) SelectEntryProfile(
	_ context.Context, _ string, _ []string, _ string, _ *models.Executor, _ *models.ExecutorProfile,
) (string, error) {
	r.calls++
	return r.profile, r.err
}

func taggedStep() *wfmodels.WorkflowStep {
	return &wfmodels.WorkflowStep{
		ID:          "step-1",
		WorkflowID:  "wf-1",
		AllowedTags: []string{"review"},
	}
}

func TestAttachPendingEntryRouteOverrideWinsWithoutQuotaSelection(t *testing.T) {
	overrides, err := models.NewWorkflowAgentOverrides("wf-1", []models.WorkflowAgentOverrideBinding{
		{StepID: "step-1", SourceProfileID: "src", ReplacementProfileID: "profile-override"},
	})
	if err != nil {
		t.Fatalf("overrides: %v", err)
	}
	selector := &recordingEntrySelector{profile: "profile-quota"}
	svc := &Service{workflowEntryProfileSelector: selector}

	ctx, err := svc.attachPendingEntryRoute(
		context.Background(), &models.Task{WorkflowID: "wf-1", WorkflowAgentOverrides: overrides}, taggedStep(), nil,
	)
	if err != nil {
		t.Fatalf("attach: %v", err)
	}
	pending, ok := entryroute.FromContext(ctx)
	if !ok || pending.AgentProfileID != "profile-override" {
		t.Fatalf("pending = %#v, want profile-override", pending)
	}
	if selector.calls != 0 {
		t.Fatalf("selector called %d times, want 0 (override wins)", selector.calls)
	}
}

func TestAttachPendingEntryRouteUsesQuotaSelection(t *testing.T) {
	selector := &recordingEntrySelector{profile: "profile-quota"}
	svc := &Service{workflowEntryProfileSelector: selector}

	ctx, err := svc.attachPendingEntryRoute(context.Background(), &models.Task{WorkflowID: "wf-1"}, taggedStep(), nil)
	if err != nil {
		t.Fatalf("attach: %v", err)
	}
	pending, ok := entryroute.FromContext(ctx)
	if !ok || pending.AgentProfileID != "profile-quota" {
		t.Fatalf("pending = %#v, want profile-quota", pending)
	}
	if selector.calls != 1 {
		t.Fatalf("selector called %d times, want 1", selector.calls)
	}
}

func TestAttachPendingEntryRouteExportedSeamFreezesTaggedStep(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	setupTestTask(t, repo)
	svc.SetWorkflowStepGetter(&fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"step-1": {ID: "step-1", WorkflowID: "wf-123", AllowedTags: []string{"review"}},
	}})
	svc.SetWorkflowEntryProfileSelector(&recordingEntrySelector{profile: "profile-frozen"})

	attached, err := svc.AttachPendingEntryRouteForStep(ctx, "task-123", "step-1")
	if err != nil {
		t.Fatalf("attach: %v", err)
	}
	pending, ok := entryroute.FromContext(attached)
	if !ok || pending.AgentProfileID != "profile-frozen" || pending.DestinationStepID != "step-1" {
		t.Fatalf("pending = %#v (ok=%v), want profile-frozen at step-1", pending, ok)
	}
}

func TestAttachPendingEntryRouteSkipsUntaggedStep(t *testing.T) {
	selector := &recordingEntrySelector{profile: "profile-quota"}
	svc := &Service{workflowEntryProfileSelector: selector}

	ctx, err := svc.attachPendingEntryRoute(context.Background(), &models.Task{WorkflowID: "wf-1"}, &wfmodels.WorkflowStep{ID: "step-1", WorkflowID: "wf-1"}, nil)
	if err != nil {
		t.Fatalf("attach: %v", err)
	}
	if _, ok := entryroute.FromContext(ctx); ok {
		t.Fatal("untagged step must not attach a pending route")
	}
	if selector.calls != 0 {
		t.Fatalf("selector called %d times, want 0", selector.calls)
	}
}

// ctxCapturingPreflight records the context the credential preflight received
// so a test can prove entry selection ran before it.
type ctxCapturingPreflight struct {
	seen context.Context
}

func (p *ctxCapturingPreflight) PreflightWorkflowStepMove(
	ctx context.Context, _ string, _ *models.TaskSession, _ *wfmodels.WorkflowStep,
) error {
	p.seen = ctx
	return nil
}

func TestMoveTaskSelectsFrozenProfileBeforeCredentialPreflight(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)
	getter := svc.workflowStepGetter.(*fakeWorkflowStepGetter)
	getter.steps["step-review-target"].AllowedTags = []string{"review"}
	createMoveTask(t, ctx, repo, "task-tagged-move", "wf-source", "step-source", nil)
	createMoveSession(t, ctx, repo, "session-tagged-move", "task-tagged-move", models.TaskSessionStateRunning, models.ReviewStatusNone)

	selector := &recordingEntrySelector{profile: "profile-frozen"}
	svc.SetWorkflowEntryProfileSelector(selector)
	preflight := &ctxCapturingPreflight{}
	svc.SetWorkflowMovePreflight(preflight)

	_, err := svc.MoveTaskWithOptions(ctx, "task-tagged-move", "wf-source", "step-review-target", 0, MoveTaskOptions{
		AllowActivePrimarySession: true,
	})
	if err != nil {
		t.Fatalf("MoveTaskWithOptions: %v", err)
	}
	if selector.calls != 1 {
		t.Fatalf("selector calls = %d, want 1", selector.calls)
	}
	pending, ok := entryroute.FromContext(preflight.seen)
	if !ok || pending.AgentProfileID != "profile-frozen" {
		t.Fatalf("preflight did not observe the pending route: %#v", pending)
	}
	if pending.DestinationStepID != "step-review-target" {
		t.Fatalf("pending destination = %q, want step-review-target", pending.DestinationStepID)
	}

	task, err := repo.GetTask(ctx, "task-tagged-move")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	route, ok := models.LoadWorkflowSessionRoute(task.Metadata)
	if !ok || route.AgentProfileID != "profile-frozen" {
		t.Fatalf("committed route = %#v, want profile-frozen", route)
	}
}
