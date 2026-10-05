package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/clarification"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedContinuationTask(t *testing.T, repo *sqliterepo.Repository, taskID string) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-" + taskID, Name: "WS", CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-" + taskID, WorkspaceID: "ws-" + taskID, Name: "WF", CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: taskID, WorkflowID: "wf-" + taskID, Title: "T", State: v1.TaskStateInProgress,
		CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "s1", TaskID: taskID, State: models.TaskSessionStateWaitingForInput,
		StartedAt: now, UpdatedAt: now,
	}))
}

func TestApplyAutoResumeHandoff_ExtractionFailurePauses(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedContinuationTask(t, repo, "t-extract")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	messages := &mockMessageCreator{}
	svc.messageCreator = messages

	session := &models.TaskSession{ID: "s1", TaskID: "t-extract", State: models.TaskSessionStateWaitingForInput}
	step := &wfmodels.WorkflowStep{ID: "step-1", Name: "Step"}

	outcome, handoff := svc.applyAutoResumeHandoff(ctx, "t-extract", session, step, "entry prompt", 7)

	require.Equal(t, autoResumePaused, outcome, "a failed extraction must pause, not fall back")
	assert.Empty(t, handoff)

	task, err := repo.GetTask(ctx, "t-extract")
	require.NoError(t, err)
	record := models.PendingContinuationRecord(task.Metadata)
	require.NotNil(t, record, "the paused entry must persist pending_continuation")
	assert.Equal(t, models.PendingContinuationCaseCold, record.Case)
	assert.Equal(t, models.PendingContinuationReasonExtractionFailed, record.Reason)
	assert.Equal(t, "entry prompt", record.EntryPrompt)
	assert.Equal(t, "step-1", record.StepID)
	assert.Equal(t, "7", record.EntryID)
	assert.NotEmpty(t, record.Stamp)

	require.Len(t, messages.sessionMessages, 1, "exactly one recovery bundle is created")
	bundle := messages.sessionMessages[0]
	assert.Equal(t, continuationRecoveryBundleType, bundle.messageType)
	assert.Equal(t, "s1", bundle.sessionID)
	assert.True(t, bundle.requestsInput)
	recovery, ok := bundle.metadata["continuation_recovery"].(*clarification.ContinuationRecoveryMeta)
	require.True(t, ok)
	assert.Equal(t, record.Stamp, recovery.Stamp)
	assert.Equal(t, models.PendingContinuationCaseCold, recovery.Case)
	assert.Equal(t, models.PendingContinuationReasonExtractionFailed, recovery.Reason)
}

func TestApplyAutoResumeHandoff_EmptyHandoffPauses(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedContinuationTask(t, repo, "t-empty")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.messageCreator = &mockMessageCreator{}
	svc.sessionlessRunner = func(context.Context, string, string) (string, error) { return "   ", nil }

	session := &models.TaskSession{ID: "s1", TaskID: "t-empty", State: models.TaskSessionStateWaitingForInput}
	step := &wfmodels.WorkflowStep{ID: "step-1", Name: "Step"}

	outcome, _ := svc.applyAutoResumeHandoff(ctx, "t-empty", session, step, "entry prompt")
	require.Equal(t, autoResumePaused, outcome)

	task, err := repo.GetTask(ctx, "t-empty")
	require.NoError(t, err)
	record := models.PendingContinuationRecord(task.Metadata)
	require.NotNil(t, record)
	assert.Equal(t, models.PendingContinuationReasonExtractionFailed, record.Reason,
		"an empty handoff maps to extraction_failed")
}

func TestResolveContinuationRecovery_StaleStampNoOp(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedContinuationTask(t, repo, "t-stale")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.messageCreator = &mockMessageCreator{}
	require.NoError(t, repo.SetTaskMetadataKey(ctx, "t-stale", models.MetaKeyPendingContinuation, &models.PendingContinuation{
		Stamp: "real", Case: models.PendingContinuationCaseCold, Reason: models.PendingContinuationReasonExtractionFailed,
		EntryPrompt: "p", SourceSessionID: "s1",
	}))

	require.NoError(t, svc.ResolveContinuationRecovery(ctx, "t-stale", "other", "continue"))

	task, err := repo.GetTask(ctx, "t-stale")
	require.NoError(t, err)
	assert.NotNil(t, models.PendingContinuationRecord(task.Metadata), "a stale stamp must not clear the record")
}

func TestContinueContinuationWithoutHandoff_ClearsRecord(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedContinuationTask(t, repo, "t-continue")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	require.NoError(t, repo.SetTaskMetadataKey(ctx, "t-continue", models.MetaKeyPendingContinuation, &models.PendingContinuation{
		Stamp: "stamp-1", Case: models.PendingContinuationCaseCold, Reason: models.PendingContinuationReasonExtractionFailed,
		EntryPrompt: "", SourceSessionID: "s1",
	}))

	require.NoError(t, svc.ResolveContinuationRecovery(ctx, "t-continue", "stamp-1", "continue"))

	task, err := repo.GetTask(ctx, "t-continue")
	require.NoError(t, err)
	assert.Nil(t, models.PendingContinuationRecord(task.Metadata), "continue clears the settled record")
}

func TestRetryContinuationRecovery_ExtractionFailureRepauses(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedContinuationTask(t, repo, "t-retry")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	messages := &mockMessageCreator{}
	svc.messageCreator = messages
	require.NoError(t, repo.SetTaskMetadataKey(ctx, "t-retry", models.MetaKeyPendingContinuation, &models.PendingContinuation{
		Stamp: "stamp-old", Case: models.PendingContinuationCaseCold, Reason: models.PendingContinuationReasonExtractionFailed,
		EntryPrompt: "entry", SourceSessionID: "s1",
	}))

	require.NoError(t, svc.ResolveContinuationRecovery(ctx, "t-retry", "stamp-old", "retry"))

	task, err := repo.GetTask(ctx, "t-retry")
	require.NoError(t, err)
	record := models.PendingContinuationRecord(task.Metadata)
	require.NotNil(t, record, "a repeated extraction failure must re-pause")
	assert.NotEqual(t, "stamp-old", record.Stamp, "the re-pause mints a new stamp")
	assert.Equal(t, models.PendingContinuationReasonExtractionFailed, record.Reason)
	require.Len(t, messages.sessionMessages, 1, "the re-pause creates a fresh bundle")
}

func TestClearPendingContinuationOnEntry_ClearsRecord(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedContinuationTask(t, repo, "t-entry")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	require.NoError(t, repo.SetTaskMetadataKey(ctx, "t-entry", models.MetaKeyPendingContinuation, &models.PendingContinuation{
		Stamp: "stamp-1", Case: models.PendingContinuationCaseCold, Reason: models.PendingContinuationReasonExtractionFailed,
		EntryPrompt: "p", SourceSessionID: "s1",
	}))

	svc.clearPendingContinuationOnEntry(ctx, "t-entry")

	task, err := repo.GetTask(ctx, "t-entry")
	require.NoError(t, err)
	assert.Nil(t, models.PendingContinuationRecord(task.Metadata))
}

// newContinuationDispatchService builds a service able to run the real
// step-entry dispatch (queue merge + handoff carry + prompt), so the
// retry/continue paths can be tested end to end.
func newContinuationDispatchService(t *testing.T, taskID, sessionID, stepID string) (*Service, *sqliterepo.Repository, *mockAgentManager) {
	t.Helper()
	repo := setupTestRepo(t)
	seedSession(t, repo, taskID, sessionID, stepID)
	seedExecutorRunning(t, repo, sessionID, taskID, "exec-1")
	ctx := context.Background()
	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	session.State = models.TaskSessionStateWaitingForInput
	session.AgentExecutionID = "exec-1"
	require.NoError(t, repo.UpdateTaskSession(ctx, session))

	stepGetter := newMockStepGetter()
	stepGetter.steps[stepID] = &wfmodels.WorkflowStep{ID: stepID, WorkflowID: "wf1", Name: "Work"}
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo, isAgentRunning: true}
	svc := createTestServiceWithAgent(repo, stepGetter, newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	svc.messageCreator = &mockMessageCreator{}
	svc.turnService = &repoTurnService{repo: repo}
	return svc, repo, agentMgr
}

// TestContinueContinuationWithoutHandoff_MergesQueuedHandoffAndCarry pins the
// D10 "never lose the entry prompt" contract: continue sends the step entry
// prompt together with the queued move hand-off and the step handoff carry,
// exactly as the normal step entry would.
func TestContinueContinuationWithoutHandoff_MergesQueuedHandoffAndCarry(t *testing.T) {
	ctx := context.Background()
	const (
		taskID    = "t-continue-dispatch"
		sessionID = "s1"
		stepID    = "step-1"
	)
	svc, repo, agentMgr := newContinuationDispatchService(t, taskID, sessionID, stepID)

	require.NoError(t, svc.messageQueue.SetAutoRun(ctx, sessionID, true))
	_, err := svc.messageQueue.QueueMessage(
		ctx, sessionID, taskID, "REVIEWER HANDOFF", "", messagequeue.QueuedByMoveTask, false, nil,
	)
	require.NoError(t, err)
	require.NoError(t, repo.SetTaskMetadataKey(ctx, taskID, models.MetaKeyStepHandoffCarry, models.StepHandoffCarryToken{
		Handoff: "CARRY TEXT", StepID: stepID, Stamp: "carry-1",
	}))
	require.NoError(t, repo.SetTaskMetadataKey(ctx, taskID, models.MetaKeyPendingContinuation, &models.PendingContinuation{
		Stamp: "stamp-1", StepID: stepID, Case: models.PendingContinuationCaseCold,
		Reason: models.PendingContinuationReasonExtractionFailed, EntryPrompt: "STEP ENTRY", SourceSessionID: sessionID,
	}))

	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	require.NoError(t, svc.ResolveContinuationRecovery(ctx, taskID, "stamp-1", clarification.ContinuationRecoveryDecisionContinue))

	agentMgr.mu.Lock()
	prompts := append([]string(nil), agentMgr.capturedPrompts...)
	agentMgr.mu.Unlock()
	require.Len(t, prompts, 1, "continue dispatches exactly one prompt")
	assert.Contains(t, prompts[0], "STEP ENTRY")
	assert.Contains(t, prompts[0], "REVIEWER HANDOFF", "the queued move hand-off must be merged")
	assert.Contains(t, prompts[0], "CARRY TEXT", "the step handoff carry must be merged")

	task, err := repo.GetTask(ctx, taskID)
	require.NoError(t, err)
	assert.Nil(t, models.PendingContinuationRecord(task.Metadata), "continue clears the settled record")
	assert.Equal(t, 0, svc.messageQueue.GetStatus(ctx, sessionID).Count, "the hand-off queue entry is consumed")
	_ = session
}

// TestRetryContinuationRecovery_ResetFailedReusesExtraction covers R7-2: a
// reset-failed retry reuses the stored handoff without another extraction,
// resets, sends handoff + entry prompt, and clears the record.
func TestRetryContinuationRecovery_ResetFailedReusesExtraction(t *testing.T) {
	ctx := context.Background()
	const (
		taskID    = "t-retry-reset"
		sessionID = "s1"
		stepID    = "step-1"
	)
	svc, repo, agentMgr := newContinuationDispatchService(t, taskID, sessionID, stepID)
	runnerCalls := 0
	svc.sessionlessRunner = func(context.Context, string, string) (string, error) {
		runnerCalls++
		return "SHOULD NOT RUN", nil
	}
	require.NoError(t, repo.SetTaskMetadataKey(ctx, taskID, models.MetaKeyPendingContinuation, &models.PendingContinuation{
		Stamp: "stamp-1", StepID: stepID, Case: models.PendingContinuationCaseCold,
		Reason: models.PendingContinuationReasonResetFailed, EntryPrompt: "STEP ENTRY",
		ExtractedHandoff: "STORED HANDOFF", SourceSessionID: sessionID,
	}))

	require.NoError(t, svc.ResolveContinuationRecovery(ctx, taskID, "stamp-1", clarification.ContinuationRecoveryDecisionRetry))

	agentMgr.mu.Lock()
	prompts := append([]string(nil), agentMgr.capturedPrompts...)
	agentMgr.mu.Unlock()
	require.Len(t, prompts, 1)
	assert.Contains(t, prompts[0], "STORED HANDOFF")
	assert.Contains(t, prompts[0], "STEP ENTRY")
	assert.Equal(t, 0, runnerCalls, "a reset-failed retry must reuse the stored extraction")

	task, err := repo.GetTask(ctx, taskID)
	require.NoError(t, err)
	assert.Nil(t, models.PendingContinuationRecord(task.Metadata), "a successful retry clears the record")
}

// TestContinueContinuationWithoutHandoff_FailedSendKeepsRecord pins that a
// failed dispatch propagates and leaves the pending record for a retry.
func TestContinueContinuationWithoutHandoff_FailedSendKeepsRecord(t *testing.T) {
	ctx := context.Background()
	const (
		taskID    = "t-continue-fail"
		sessionID = "s1"
		stepID    = "step-1"
	)
	svc, repo, agentMgr := newContinuationDispatchService(t, taskID, sessionID, stepID)
	agentMgr.mu.Lock()
	agentMgr.promptErr = assert.AnError
	agentMgr.mu.Unlock()
	require.NoError(t, repo.SetTaskMetadataKey(ctx, taskID, models.MetaKeyPendingContinuation, &models.PendingContinuation{
		Stamp: "stamp-1", StepID: stepID, Case: models.PendingContinuationCaseCold,
		Reason: models.PendingContinuationReasonExtractionFailed, EntryPrompt: "STEP ENTRY", SourceSessionID: sessionID,
	}))

	err := svc.ResolveContinuationRecovery(ctx, taskID, "stamp-1", clarification.ContinuationRecoveryDecisionContinue)
	require.Error(t, err, "a failed send must propagate")

	task, err2 := repo.GetTask(ctx, taskID)
	require.NoError(t, err2)
	assert.NotNil(t, models.PendingContinuationRecord(task.Metadata), "a failed send keeps the record for a retry")
}
