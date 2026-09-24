package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/entryroute"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// seedTaggedRouteTask creates a task at step-a and moves it to a tagged step-b
// through the repository write path so the bounded route is persisted exactly
// as production does.
func seedTaggedRouteTask(t *testing.T, ctx context.Context, repo interface {
	CreateTask(context.Context, *models.Task) error
	GetTask(context.Context, string) (*models.Task, error)
	UpdateTask(context.Context, *models.Task) error
}, startPolicy string) *models.Task {
	t.Helper()
	task := &models.Task{
		ID: "task-tagged-route", WorkspaceID: "ws-1", WorkflowID: "wf-1",
		WorkflowStepID: "step-a", Title: "Tagged", Priority: "medium",
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	moveCtx := entryroute.WithPendingRoute(ctx, entryroute.PendingRoute{
		DestinationStepID: "step-b",
		AgentProfileID:    "profile-frozen",
		StartPolicy:       startPolicy,
	})
	moved, err := repo.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	moved.WorkflowStepID = "step-b"
	if err := repo.UpdateTask(moveCtx, moved); err != nil {
		t.Fatalf("move task: %v", err)
	}
	frozen, err := repo.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	return frozen
}

func taggedEntryStep(startPolicy models.WorkflowProfileSessionStartPolicy) *wfmodels.WorkflowStep {
	return &wfmodels.WorkflowStep{
		ID:                        "step-b",
		WorkflowID:                "wf-1",
		AllowedTags:               []string{"review"},
		ProfileSessionStartPolicy: startPolicy,
	}
}

func TestResolveTaggedEntryProfileUsesExactFrozenRoute(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	frozen := seedTaggedRouteTask(t, ctx, repo, "new")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	got, err := svc.resolveStepAgentProfileForTaskError(ctx, frozen, taggedEntryStep(models.WorkflowProfileSessionStartPolicyNew))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != "profile-frozen" {
		t.Fatalf("resolved = %q, want profile-frozen", got)
	}
}

func TestResolveTaggedEntryProfileRequiresMatchingStartPolicy(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	frozen := seedTaggedRouteTask(t, ctx, repo, "new")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	_, err := svc.resolveStepAgentProfileForTaskError(ctx, frozen, taggedEntryStep(models.WorkflowProfileSessionStartPolicyReuse))
	if !errors.Is(err, entryroute.ErrNoFrozenProfile) {
		t.Fatalf("err = %v, want ErrNoFrozenProfile for start-policy mismatch", err)
	}
}

func TestResolveTaggedEntryProfileMissingRouteFailsClosed(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	task := &models.Task{
		ID: "task-no-route", WorkspaceID: "ws-1", WorkflowID: "wf-1",
		WorkflowStepID: "step-b", Title: "NoRoute", Priority: "medium",
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	loaded, err := repo.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}

	_, err = svc.resolveStepAgentProfileForTaskError(ctx, loaded, taggedEntryStep(models.WorkflowProfileSessionStartPolicyNew))
	if !errors.Is(err, entryroute.ErrNoFrozenProfile) {
		t.Fatalf("err = %v, want ErrNoFrozenProfile", err)
	}
}

// recordingOrchestratorEntrySelector counts selector calls so a test can prove
// the no-route launch fallback never re-ranks quota.
type recordingOrchestratorEntrySelector struct{ calls int }

func (r *recordingOrchestratorEntrySelector) SelectEntryProfile(
	_ context.Context, _ string, _ []string, _ string, _ *models.Executor, _ *models.ExecutorProfile,
) (string, error) {
	r.calls++
	return "selector-should-not-run", nil
}

// TestResolveTaggedEntryProfileFallsBackToStepProfileWithoutRoute covers a task
// that entered a step before allowed_tags were added: no frozen route exists,
// so launch/restart/replay/reuse fall back to the step's configured profile
// without re-ranking quota or persisting a route
// (AC-AGENTS-TAGGED-QUOTA-SELECTION-001.16).
func TestResolveTaggedEntryProfileFallsBackToStepProfileWithoutRoute(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	task := &models.Task{
		ID: "task-pretags", WorkspaceID: "ws-1", WorkflowID: "wf-1",
		WorkflowStepID: "step-b", Title: "PreTags", Priority: "medium",
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	selector := &recordingOrchestratorEntrySelector{}
	svc.SetWorkflowEntryProfileSelector(selector)
	loaded, err := repo.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}

	step := taggedEntryStep(models.WorkflowProfileSessionStartPolicyNew)
	step.AgentProfileID = "  step-fallback-profile  "
	got, err := svc.resolveStepAgentProfileForTaskError(ctx, loaded, step)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != "step-fallback-profile" {
		t.Fatalf("resolved = %q, want the trimmed step fallback", got)
	}
	if selector.calls != 0 {
		t.Fatalf("selector called %d times, want 0 for the no-route fallback", selector.calls)
	}
	if _, ok := models.LoadWorkflowSessionRoute(loaded.Metadata); ok {
		t.Fatal("the no-route fallback must not persist a workflow session route")
	}
}

// TestResolveTaggedEntryProfileTaskOverrideWinsWithoutRoute proves the task
// fixed-step override is applied before the tagged-route requirement.
func TestResolveTaggedEntryProfileTaskOverrideWinsWithoutRoute(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	task := &models.Task{
		ID: "task-override", WorkspaceID: "ws-1", WorkflowID: "wf-1",
		WorkflowStepID: "step-b", Title: "Override", Priority: "medium",
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	overrides, err := models.NewWorkflowAgentOverrides("wf-1", []models.WorkflowAgentOverrideBinding{
		{StepID: "step-b", SourceProfileID: "step-a-profile", ReplacementProfileID: "profile-override"},
	})
	if err != nil {
		t.Fatalf("build overrides: %v", err)
	}
	task.WorkflowAgentOverrides = overrides
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	step := taggedEntryStep(models.WorkflowProfileSessionStartPolicyNew)
	step.AgentProfileID = "step-fallback-profile"
	got, err := svc.resolveStepAgentProfileForTaskError(ctx, task, step)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != "profile-override" {
		t.Fatalf("resolved = %q, want profile-override", got)
	}
}

func TestResolveTaggedEntryProfileUsesPendingRouteOnContext(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	task := &models.Task{
		ID: "task-pending-route", WorkspaceID: "ws-1", WorkflowID: "wf-1",
		WorkflowStepID: "step-b", Title: "Pending", Priority: "medium",
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	loaded, err := repo.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	pendingCtx := entryroute.WithPendingRoute(ctx, entryroute.PendingRoute{
		DestinationStepID: "step-b",
		AgentProfileID:    "profile-pending",
		StartPolicy:       "new",
	})
	got, err := svc.resolveStepAgentProfileForTaskError(pendingCtx, loaded, taggedEntryStep(models.WorkflowProfileSessionStartPolicyNew))
	if err != nil {
		t.Fatalf("resolve with pending route: %v", err)
	}
	if got != "profile-pending" {
		t.Fatalf("resolved = %q, want profile-pending", got)
	}
}
