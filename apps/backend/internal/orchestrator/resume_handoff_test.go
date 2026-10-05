package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	agentusage "github.com/kandev/kandev/internal/agent/usage"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatHandoffTranscript(t *testing.T) {
	// Empty messages
	assert.Equal(t, "", formatHandoffTranscript(nil))
	assert.Equal(t, "", formatHandoffTranscript([]*models.Message{}))

	// Mixed message types and authors
	msgs := []*models.Message{
		{
			Type:       models.MessageTypeMessage,
			AuthorType: models.MessageAuthorUser,
			Content:    "Please implement feature X",
		},
		{
			Type:       models.MessageTypeToolCall,
			AuthorType: models.MessageAuthorAgent,
			Content:    `{"tool": "read_file"}`,
		},
		{
			Type:       models.MessageTypeContent,
			AuthorType: models.MessageAuthorAgent,
			Content:    "I have implemented feature X.",
		},
		{
			Type:       "", // Default should be treated as "message"
			AuthorType: models.MessageAuthorUser,
			Content:    "Looks good, now add tests.",
		},
	}

	expected := "User: Please implement feature X\n\nAgent: I have implemented feature X.\n\nUser: Looks good, now add tests."
	assert.Equal(t, expected, formatHandoffTranscript(msgs))
}

func TestComposeResumeHandoffPrompt(t *testing.T) {
	handoff := "## Summary\n- Done task 1\n- In progress task 2"

	// Without instructions
	assert.Equal(t, handoff, composeResumeHandoffPrompt(handoff, ""))
	assert.Equal(t, handoff, composeResumeHandoffPrompt(handoff, "   \n\t  "))

	// With instructions
	instructions := "Now proceed with the unit tests."
	expected := handoff + "\n\n## Additional instructions\n\n" + instructions
	assert.Equal(t, expected, composeResumeHandoffPrompt(handoff, instructions))
	assert.Equal(t, expected, composeResumeHandoffPrompt(handoff, "  "+instructions+"  \n"))
}

func TestExtractResumeHandoff(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	ctx := context.Background()

	// 1. Runner not configured
	_, err := svc.extractResumeHandoff(ctx, "session1")
	require.Error(t, err)

	// Configure runner
	var invokedWithAgentID string
	var invokedWithTranscript string
	runnerResponse := "Extracted facts"
	var runnerErr error

	svc.SetSessionlessUtilityRunner(func(_ context.Context, agentID, transcript string) (string, error) {
		invokedWithAgentID = agentID
		invokedWithTranscript = transcript
		return runnerResponse, runnerErr
	})

	// 2. Empty session messages
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)
	_, err = svc.extractResumeHandoff(ctx, "session1")
	require.ErrorIs(t, err, ErrResumeHandoffEmpty)

	// Seed turn and message
	now := time.Now().UTC()
	err = repo.CreateTurn(ctx, &models.Turn{ID: "turn1", TaskSessionID: "session1", TaskID: "task1", StartedAt: now})
	require.NoError(t, err)

	err = repo.CreateMessage(ctx, &models.Message{
		ID:            "msg1",
		TaskSessionID: "session1",
		TaskID:        "task1",
		TurnID:        "turn1",
		AuthorType:    models.MessageAuthorUser,
		Content:       "Hello world",
		Type:          models.MessageTypeMessage,
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	require.NoError(t, err)

	// 3. Successful extraction
	handoff, err := svc.extractResumeHandoff(ctx, "session1")
	require.NoError(t, err)
	assert.Equal(t, "Extracted facts", handoff)
	assert.Equal(t, ResumeHandoffAgentID, invokedWithAgentID)
	assert.Equal(t, "User: Hello world", invokedWithTranscript)

	// 4. Runner returns error
	runnerErr = errors.New("utility timeout")
	_, err = svc.extractResumeHandoff(ctx, "session1")
	require.ErrorIs(t, err, ErrExtractionFailed)

	// 5. Runner returns blank text
	runnerErr = nil
	runnerResponse = "   \n  "
	_, err = svc.extractResumeHandoff(ctx, "session1")
	require.ErrorIs(t, err, ErrResumeHandoffEmpty)
}

func TestResumeWithHandoff_NotEligible(t *testing.T) {
	ctx := context.Background()

	t.Run("session running", func(t *testing.T) {
		repo := setupTestRepo(t)
		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		seedTaskAndSession(t, repo, "task-running", "session-running", models.TaskSessionStateRunning)
		_, err := svc.ResumeWithHandoff(ctx, "session-running", "")
		require.ErrorIs(t, err, ErrSessionNotEligible)
	})

	t.Run("session passthrough", func(t *testing.T) {
		repo := setupTestRepo(t)
		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		seedTaskAndSession(t, repo, "task-passthrough", "session-passthrough", models.TaskSessionStateWaitingForInput)
		sess, err := repo.GetTaskSession(ctx, "session-passthrough")
		require.NoError(t, err)
		sess.IsPassthrough = true
		require.NoError(t, repo.UpdateTaskSession(ctx, sess))
		_, err = svc.ResumeWithHandoff(ctx, "session-passthrough", "")
		require.ErrorIs(t, err, ErrSessionNotEligible)
	})

	t.Run("session no executor running row", func(t *testing.T) {
		repo := setupTestRepo(t)
		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		seedTaskAndSession(t, repo, "task-no-exec", "session-no-exec", models.TaskSessionStateWaitingForInput)
		_, err := svc.ResumeWithHandoff(ctx, "session-no-exec", "")
		require.ErrorIs(t, err, ErrSessionNotEligible)
	})
}

type fakeUsageProviderForHandoff struct {
	remaining map[string]int
}

func (f *fakeUsageProviderForHandoff) GetUsage(_ context.Context, profileID string) (*agentusage.ProviderUsage, error) {
	rem, ok := f.remaining[profileID]
	if !ok {
		return nil, nil
	}
	utilPct := 100 - rem
	return &agentusage.ProviderUsage{
		Provider: "test",
		Windows: []agentusage.UtilizationWindow{
			{Label: "5h", UtilizationPct: float64(utilPct)},
		},
	}, nil
}

func TestResumeWithHandoff_UnavailableRejected(t *testing.T) {
	repo := setupTestRepo(t)
	taskRepo := newMockTaskRepo()
	agentMgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	ctx := context.Background()

	seedTaskAndSession(t, repo, "task1", "session-unavail", models.TaskSessionStateWaitingForInput)
	sess, err := repo.GetTaskSession(ctx, "session-unavail")
	require.NoError(t, err)
	sess.AgentProfileID = "profile-exhausted"
	sess.AgentExecutionID = "exec-unavail"
	require.NoError(t, repo.UpdateTaskSession(ctx, sess))
	seedExecutorRunning(t, repo, sess.ID, sess.TaskID, "exec-unavail")

	// Set up profile execution resolver with exhausted quota
	resolver := agentruntime.NewProfileExecutionResolver(nil, nil, true)
	resolver.SetUsageProvider(&fakeUsageProviderForHandoff{
		remaining: map[string]int{"profile-exhausted": 0},
	})
	svc.SetProfileExecutionResolver(resolver)

	// Configure runner to ensure it is not called
	runnerCalled := false
	svc.SetSessionlessUtilityRunner(func(_ context.Context, _, _ string) (string, error) {
		runnerCalled = true
		return "facts", nil
	})

	_, err = svc.ResumeWithHandoff(ctx, "session-unavail", "")
	require.ErrorIs(t, err, ErrProviderUnavailable)
	assert.False(t, runnerCalled, "runner must not be called when provider is unavailable")
	assert.Empty(t, agentMgr.restartProcessCalls, "context reset must not be called")
}

func TestResumeWithHandoff_ExtractionFailureNoReset(t *testing.T) {
	repo := setupTestRepo(t)
	taskRepo := newMockTaskRepo()
	agentMgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	ctx := context.Background()

	seedTaskAndSession(t, repo, "task1", "session-fail", models.TaskSessionStateWaitingForInput)
	sess, err := repo.GetTaskSession(ctx, "session-fail")
	require.NoError(t, err)
	sess.AgentExecutionID = "exec-fail"
	require.NoError(t, repo.UpdateTaskSession(ctx, sess))
	seedExecutorRunning(t, repo, sess.ID, sess.TaskID, "exec-fail")

	// Runner fails
	svc.SetSessionlessUtilityRunner(func(_ context.Context, _, _ string) (string, error) {
		return "", errors.New("extraction timeout")
	})

	_, err = svc.ResumeWithHandoff(ctx, "session-fail", "")
	require.ErrorIs(t, err, ErrExtractionFailed)
	assert.Empty(t, agentMgr.restartProcessCalls, "context reset must not be called on extraction failure")
}

func TestResumeWithHandoff_OrderExtractResetSend(t *testing.T) {
	repo := setupTestRepo(t)
	taskRepo := newMockTaskRepo()
	agentMgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	ctx := context.Background()

	seedTaskAndSession(t, repo, "task1", "session-order", models.TaskSessionStateWaitingForInput)
	sess, err := repo.GetTaskSession(ctx, "session-order")
	require.NoError(t, err)
	sess.AgentExecutionID = "exec-order"
	require.NoError(t, repo.UpdateTaskSession(ctx, sess))
	seedExecutorRunning(t, repo, sess.ID, sess.TaskID, "exec-order")

	// Seed turn and message so transcript is non-empty
	now := time.Now().UTC()
	err = repo.CreateTurn(ctx, &models.Turn{ID: "turn1", TaskSessionID: "session-order", TaskID: "task1", StartedAt: now})
	require.NoError(t, err)

	err = repo.CreateMessage(ctx, &models.Message{
		ID:            "msg1",
		TaskSessionID: "session-order",
		TaskID:        "task1",
		TurnID:        "turn1",
		AuthorType:    models.MessageAuthorUser,
		Content:       "Initial problem",
		Type:          models.MessageTypeMessage,
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	require.NoError(t, err)

	var callOrder []string
	svc.SetSessionlessUtilityRunner(func(_ context.Context, _, _ string) (string, error) {
		callOrder = append(callOrder, "extract")
		return "## Facts\n- Step 1 done", nil
	})

	agentMgr.promptAgentFunc = func(_ context.Context, _, prompt string, _ []v1.MessageAttachment, _ bool) (*executor.PromptResult, error) {
		callOrder = append(callOrder, "prompt")
		return &executor.PromptResult{AgentMessage: prompt}, nil
	}

	result, err := svc.ResumeWithHandoff(ctx, "session-order", "Keep going with step 2")
	require.NoError(t, err)
	assert.True(t, result.Sent)
	assert.Equal(t, "## Facts\n- Step 1 done", result.Handoff)
	expectedPrompt := "## Facts\n- Step 1 done\n\n## Additional instructions\n\nKeep going with step 2"
	assert.Equal(t, expectedPrompt, result.Prompt)

	// Verify order: extract was called first, then reset, then prompt
	require.Len(t, callOrder, 2)
	assert.Equal(t, "extract", callOrder[0])
	assert.Equal(t, "prompt", callOrder[1])
	assert.Contains(t, agentMgr.restartProcessCalls, "exec-order")
}

func TestResumeWithHandoff_SendFailureAfterReset(t *testing.T) {
	repo := setupTestRepo(t)
	taskRepo := newMockTaskRepo()
	agentMgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	ctx := context.Background()

	seedTaskAndSession(t, repo, "task1", "session-send-fail", models.TaskSessionStateWaitingForInput)
	sess, err := repo.GetTaskSession(ctx, "session-send-fail")
	require.NoError(t, err)
	sess.AgentExecutionID = "exec-send-fail"
	require.NoError(t, repo.UpdateTaskSession(ctx, sess))
	seedExecutorRunning(t, repo, sess.ID, sess.TaskID, "exec-send-fail")

	now := time.Now().UTC()
	err = repo.CreateTurn(ctx, &models.Turn{ID: "turn1", TaskSessionID: "session-send-fail", TaskID: "task1", StartedAt: now})
	require.NoError(t, err)

	err = repo.CreateMessage(ctx, &models.Message{
		ID:            "msg1",
		TaskSessionID: "session-send-fail",
		TaskID:        "task1",
		TurnID:        "turn1",
		AuthorType:    models.MessageAuthorUser,
		Content:       "Initial text",
		Type:          models.MessageTypeMessage,
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	require.NoError(t, err)

	svc.SetSessionlessUtilityRunner(func(_ context.Context, _, _ string) (string, error) {
		return "Extracted handoff", nil
	})

	// Make prompt send fail
	agentMgr.promptErr = errors.New("agentctl connection refused")

	result, err := svc.ResumeWithHandoff(ctx, "session-send-fail", "Optional instruction")
	require.NoError(t, err, "send failure after reset must return nil error and Sent=false")
	assert.False(t, result.Sent)
	assert.Equal(t, "Extracted handoff", result.Handoff)
	assert.Equal(t, "Extracted handoff\n\n## Additional instructions\n\nOptional instruction", result.Prompt)
	assert.Contains(t, agentMgr.restartProcessCalls, "exec-send-fail")
}
