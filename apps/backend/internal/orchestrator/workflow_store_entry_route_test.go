package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/entryroute"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// promotionCaptureRepo records the context each promotion write receives.
type promotionCaptureRepo struct {
	sessionExecutorStore
	capturedCtx   context.Context
	claimed       bool
	nextCandidate *models.Task
}

func (r *promotionCaptureRepo) PromoteQueuedTaskIfWorkflowStepHasCapacity(
	ctx context.Context, _ *models.Task, _, _ string, _ int,
) (bool, error) {
	r.capturedCtx = ctx
	return r.claimed, nil
}

func (r *promotionCaptureRepo) NextQueuedTaskForStepExcluding(
	context.Context, string, string, []string,
) (*models.Task, error) {
	return r.nextCandidate, nil
}

func (r *promotionCaptureRepo) GetActiveTaskSessionByTaskID(context.Context, string) (*models.TaskSession, error) {
	return nil, nil
}

func attachingEntryRoute(profileID string) entryRouteAttacher {
	return func(ctx context.Context, _, _ string, step *wfmodels.WorkflowStep) (context.Context, error) {
		return entryroute.WithPendingRoute(ctx, entryroute.PendingRoute{
			DestinationStepID: step.ID,
			AgentProfileID:    profileID,
			StartPolicy:       "reuse",
		}), nil
	}
}

func failingEntryRouteAttacher(calls *int) entryRouteAttacher {
	return func(ctx context.Context, _, _ string, _ *wfmodels.WorkflowStep) (context.Context, error) {
		*calls++
		return ctx, errors.New("selector unavailable")
	}
}

// TestPromoteSameStepKeepsFrozenRoute proves same-step WIP admission never
// re-ranks a committed entry: the attacher is not called, the promotion still
// succeeds, and the route frozen at genesis is unchanged
// (AC-AGENTS-TAGGED-QUOTA-SELECTION-001.11).
func TestPromoteSameStepKeepsFrozenRoute(t *testing.T) {
	stepGetter := newMockStepGetter()
	step := &wfmodels.WorkflowStep{ID: "step-2", WorkflowID: "wf-1", Position: 2, AllowedTags: []string{"review"}}
	stepGetter.steps[step.ID] = step
	repo := &promotionCaptureRepo{claimed: true}
	store := newWorkflowStore(repo, stepGetter, nil, noopPublisher, testLogger(), &operationLedger{})
	attachCalls := 0
	store.setEntryRouteAttacher(failingEntryRouteAttacher(&attachCalls))

	frozen := models.WorkflowSessionRoute{
		OperationID:       "workflow-session:t1:step-2:entry:00000000000000000001:profile::reuse",
		DestinationStepID: "step-2",
		EntryIdentity:     "entry:00000000000000000001",
		TargetKind:        entryroute.TargetKindProfile,
		AgentProfileID:    "profile-frozen",
		Phase:             "committed",
	}
	candidate := &models.Task{
		ID: "t1", WorkflowID: "wf-1", WorkflowStepID: "step-2",
		State: v1.TaskStateInProgress, QueuedForStepID: "step-2",
		Metadata: map[string]interface{}{models.MetaKeyWorkflowSessionRoute: frozen},
	}
	if !store.promoteSameStepTask(context.Background(), candidate, step, 0, map[string]struct{}{}, nil, nil) {
		t.Fatal("promoteSameStepTask = false, want true")
	}
	if attachCalls != 0 {
		t.Fatalf("entry route attacher called %d times, want 0 for same-step promotion", attachCalls)
	}
	got, ok := models.LoadWorkflowSessionRoute(candidate.Metadata)
	if !ok || got != frozen {
		t.Fatalf("frozen route = %#v (ok=%v), want unchanged %#v", got, ok, frozen)
	}
}

func TestPullFeederTaskAttachesPendingRouteBeforeWrite(t *testing.T) {
	stepGetter := newMockStepGetter()
	vacated := &wfmodels.WorkflowStep{
		ID: "step-2", WorkflowID: "wf-1", Position: 2, PullFromStepID: "step-0",
		AllowedTags: []string{"review"},
	}
	stepGetter.steps[vacated.ID] = vacated
	repo := &promotionCaptureRepo{
		claimed:       true,
		nextCandidate: &models.Task{ID: "t1", WorkflowID: "wf-0", WorkflowStepID: "feeder", State: v1.TaskStateInProgress},
	}
	moved := taskMovedPublisher(func(context.Context, *models.Task, string, string, string, string) {})
	store := newWorkflowStore(repo, stepGetter, nil, noopPublisher, testLogger(), &operationLedger{}, moved)
	store.setEntryRouteAttacher(attachingEntryRoute("profile-frozen"))

	if !store.pullOneFeederTask(context.Background(), nil, nil, vacated, 0, map[string]struct{}{}) {
		t.Fatal("pullOneFeederTask = false, want true")
	}
	pending, ok := entryroute.FromContext(repo.capturedCtx)
	if !ok || pending.AgentProfileID != "profile-frozen" {
		t.Fatalf("feeder promotion write ctx pending = %#v, want profile-frozen", pending)
	}
}
