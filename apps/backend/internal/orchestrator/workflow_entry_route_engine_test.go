package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/engine"
	"github.com/kandev/kandev/internal/workflow/entryroute"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type recordingOrchestratorSelector struct {
	calls           int
	profile         string
	err             error
	executor        *models.Executor
	executorProfile *models.ExecutorProfile
}

func (r *recordingOrchestratorSelector) SelectEntryProfile(
	_ context.Context, _ string, _ []string, _ string, executor *models.Executor, executorProfile *models.ExecutorProfile,
) (string, error) {
	r.calls++
	r.executor = executor
	r.executorProfile = executorProfile
	return r.profile, r.err
}

func TestAttachEngineEntryRouteUsesSelectorForTaggedStep(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	now := time.Now().UTC()
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "t1", WorkspaceID: "ws1", WorkflowID: "wf1", WorkflowStepID: "step1",
		Title: "T", Priority: "medium", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	selector := &recordingOrchestratorSelector{profile: "profile-frozen"}
	svc := &Service{repo: repo, logger: testLogger(), workflowEntryProfileSelector: selector}

	step := &wfmodels.WorkflowStep{ID: "step2", WorkflowID: "wf1", AllowedTags: []string{"review"}}
	attached, err := svc.attachEngineEntryRoute(ctx, "t1", "", step)
	if err != nil {
		t.Fatalf("attachEngineEntryRoute: %v", err)
	}
	if selector.calls != 1 {
		t.Fatalf("selector calls = %d, want 1", selector.calls)
	}
	pending, ok := entryroute.FromContext(attached)
	if !ok || pending.AgentProfileID != "profile-frozen" || pending.DestinationStepID != "step2" {
		t.Fatalf("pending = %#v, want profile-frozen at step2", pending)
	}
}

func TestAttachEngineEntryRouteSkipsUntaggedStep(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	selector := &recordingOrchestratorSelector{profile: "profile-frozen"}
	svc := &Service{repo: repo, logger: testLogger(), workflowEntryProfileSelector: selector}

	attached, err := svc.attachEngineEntryRoute(ctx, "", "", &wfmodels.WorkflowStep{ID: "step2", WorkflowID: "wf1"})
	if err != nil {
		t.Fatalf("attachEngineEntryRoute: %v", err)
	}
	if _, ok := entryroute.FromContext(attached); ok {
		t.Fatal("untagged step must not attach a pending route")
	}
	if selector.calls != 0 {
		t.Fatalf("selector calls = %d, want 0", selector.calls)
	}
}

func TestGuardedTransitionPersistsTaggedEntryRoute(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	now := time.Now().UTC()

	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "t1", WorkspaceID: "ws1", WorkflowID: "wf1", WorkflowStepID: "step1",
		Title: "T", Description: "T", State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	session := &models.TaskSession{
		ID: "s1", TaskID: "t1", AgentProfileID: "profile-a",
		State: models.TaskSessionStateRunning, IsPrimary: true, StartedAt: now, UpdatedAt: now,
	}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatalf("create session: %v", err)
	}

	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{ID: "step1", WorkflowID: "wf1", Position: 1}
	stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
		ID: "step2", WorkflowID: "wf1", Position: 2, AllowedTags: []string{"review"},
	}
	taskRepo := newMockTaskRepo()
	taskRepo.tasks["t1"] = &v1.Task{ID: "t1", WorkspaceID: "ws1", WorkflowID: "wf1", Title: "T", State: v1.TaskStateInProgress}
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo, isAgentRunning: true}
	log := testLogger()
	exec := executor.NewExecutor(agentMgr, repo, log, executor.ExecutorConfig{})
	selector := &recordingOrchestratorSelector{profile: "profile-frozen"}
	svc := &Service{
		logger: log, repo: repo, workflowStepGetter: stepGetter, taskRepo: taskRepo, agentManager: agentMgr,
		messageQueue: messagequeue.NewServiceMemory(log), executor: exec,
		workflowStore:                newWorkflowStore(repo, stepGetter, agentMgr, noopPublisher, log, &operationLedger{}),
		workflowEntryProfileSelector: selector,
	}

	applied, err := svc.applyGuardedTransitionLifecycle(ctx, "t1", "s1", "step1", "step2", engine.TriggerOnTurnStart)
	if err != nil {
		t.Fatalf("applyGuardedTransitionLifecycle: %v", err)
	}
	if !applied {
		t.Fatal("guarded transition did not apply")
	}
	stored, err := repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if stored.WorkflowStepID != "step2" {
		t.Fatalf("step = %q, want step2", stored.WorkflowStepID)
	}
	route, ok := models.LoadWorkflowSessionRoute(stored.Metadata)
	if !ok || route.AgentProfileID != "profile-frozen" || route.DestinationStepID != "step2" {
		t.Fatalf("route = %#v, want profile-frozen at step2", route)
	}
	if route.EntryIdentity == "" || route.TargetKind != "profile" {
		t.Fatalf("route identity/kind = (%q, %q), want entry identity and profile kind", route.EntryIdentity, route.TargetKind)
	}
}

// engineRouteStubRepo embeds the orchestrator repo interface and implements
// only the reads attachEngineEntryRoute uses.
type engineRouteStubRepo struct {
	sessionExecutorStore
	task     *models.Task
	session  *models.TaskSession
	executor *models.Executor
	profile  *models.ExecutorProfile
	taskErr  error
}

func (r *engineRouteStubRepo) GetTask(context.Context, string) (*models.Task, error) {
	if r.taskErr != nil {
		return nil, r.taskErr
	}
	return r.task, nil
}

func (r *engineRouteStubRepo) GetTaskSession(context.Context, string) (*models.TaskSession, error) {
	return r.session, nil
}

func (r *engineRouteStubRepo) GetExecutor(context.Context, string) (*models.Executor, error) {
	return r.executor, nil
}

func (r *engineRouteStubRepo) GetExecutorProfile(context.Context, string) (*models.ExecutorProfile, error) {
	return r.profile, nil
}

func TestAttachEngineEntryRoutePrefersSessionExecutor(t *testing.T) {
	executor := &models.Executor{ID: "exec1", Type: models.ExecutorTypeSSH}
	executorProfile := &models.ExecutorProfile{ID: "ep1"}
	repo := &engineRouteStubRepo{
		task:     &models.Task{ID: "t1", WorkflowID: "wf1"},
		session:  &models.TaskSession{ID: "s1", ExecutorID: "exec1", ExecutorProfileID: "ep1"},
		executor: executor,
		profile:  executorProfile,
	}
	selector := &recordingOrchestratorSelector{profile: "profile-frozen"}
	svc := &Service{repo: repo, logger: testLogger(), workflowEntryProfileSelector: selector}

	step := &wfmodels.WorkflowStep{ID: "step2", WorkflowID: "wf1", AllowedTags: []string{"review"}}
	if _, err := svc.attachEngineEntryRoute(context.Background(), "t1", "s1", step); err != nil {
		t.Fatalf("attachEngineEntryRoute: %v", err)
	}
	if selector.executor != executor || selector.executorProfile != executorProfile {
		t.Fatalf("selector executor = (%#v, %#v), want the session's executor", selector.executor, selector.executorProfile)
	}
}

func TestAttachEngineEntryRouteFailsClosedOnTaskReadError(t *testing.T) {
	repo := &engineRouteStubRepo{taskErr: errors.New("transient read failure")}
	selector := &recordingOrchestratorSelector{profile: "profile-frozen"}
	svc := &Service{repo: repo, logger: testLogger(), workflowEntryProfileSelector: selector}

	step := &wfmodels.WorkflowStep{ID: "step2", WorkflowID: "wf1", AllowedTags: []string{"review"}}
	if _, err := svc.attachEngineEntryRoute(context.Background(), "t1", "s1", step); err == nil {
		t.Fatal("attachEngineEntryRoute must fail closed on a task read error")
	}
	if selector.calls != 0 {
		t.Fatalf("selector calls = %d, want 0 (task read failed first)", selector.calls)
	}
}
