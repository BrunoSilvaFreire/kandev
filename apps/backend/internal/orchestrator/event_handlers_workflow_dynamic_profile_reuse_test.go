package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	agentusage "github.com/kandev/kandev/internal/agent/usage"
	"github.com/kandev/kandev/internal/steptelemetry"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/stretchr/testify/require"
)

// A dynamic (virtual) step profile resolves to lifecycle.ErrVirtualProfile from
// the store-backed resolver. Exact-model identity for such a profile belongs to
// the dynamic conductor, so the orchestrator's store-resolver drift check must
// treat it as "no constraint here" instead of aborting workflow entry after the
// move has already committed.

func TestPrepareWorkflowStepSession_DynamicProfileReuseParkedSession(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyReuse, models.WorkflowProfileSessionEndPolicyPark)
	fixture.agentMgr.resolveProfileErr = fmt.Errorf("profile dyn-profile: %w", lifecycle.ErrVirtualProfile)

	parked := &models.TaskSession{
		ID: "session-parked-dyn", TaskID: "t1", AgentProfileID: "dyn-profile",
		ExecutorID: "exec-local", ExecutorProfileID: "ep1", TaskEnvironmentID: "env-1",
		State: models.TaskSessionStateWaitingForInput, StartedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	require.NoError(t, fixture.repo.CreateTaskSession(ctx, parked))
	seedExecutorRunning(t, fixture.repo, parked.ID, parked.TaskID, "execution-parked-dyn")

	targetStep := &wfmodels.WorkflowStep{
		ID: "step-work", WorkflowID: "wf1", AgentProfileID: "dyn-profile",
		ProfileSessionStartPolicy: models.WorkflowProfileSessionStartPolicyReuse,
	}
	sourceStep := &wfmodels.WorkflowStep{
		ID: "step-source", WorkflowID: "wf1", AgentProfileID: "profile-a",
		ProfileSessionEndPolicy: models.WorkflowProfileSessionEndPolicyPark,
	}

	workSession, switched, err := fixture.svc.prepareWorkflowStepSession(ctx, "t1", fixture.current, targetStep, sourceStep)
	require.NoError(t, err)
	require.True(t, switched)
	require.NotNil(t, workSession)
	require.Equal(t, parked.ID, workSession.ID)
}

func TestPrepareWorkflowStepSession_DynamicProfileKeepsCurrentSession(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyReuse, models.WorkflowProfileSessionEndPolicyPark)
	fixture.agentMgr.resolveProfileErr = fmt.Errorf("profile dyn-profile: %w", lifecycle.ErrVirtualProfile)
	fixture.current.AgentProfileID = "dyn-profile"
	require.NoError(t, fixture.repo.UpdateTaskSession(ctx, fixture.current))

	targetStep := &wfmodels.WorkflowStep{
		ID: "step-work", WorkflowID: "wf1", AgentProfileID: "dyn-profile",
		ProfileSessionStartPolicy: models.WorkflowProfileSessionStartPolicyReuse,
	}
	sourceStep := &wfmodels.WorkflowStep{
		ID: "step-source", WorkflowID: "wf1", AgentProfileID: "dyn-profile",
	}

	workSession, switched, err := fixture.svc.prepareWorkflowStepSession(ctx, "t1", fixture.current, targetStep, sourceStep)
	require.NoError(t, err)
	require.False(t, switched)
	require.NotNil(t, workSession)
	require.Equal(t, fixture.current.ID, workSession.ID)
}

func TestSessionHasUnauthorizedExactModelDrift_VirtualProfile(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyReuse, models.WorkflowProfileSessionEndPolicyPark)

	fixture.agentMgr.resolveProfileErr = fmt.Errorf("profile dyn-profile: %w", lifecycle.ErrVirtualProfile)
	drifted, err := fixture.svc.sessionHasUnauthorizedExactModelDrift(ctx, fixture.current, "dyn-profile")
	require.NoError(t, err)
	require.False(t, drifted)

	fixture.agentMgr.resolveProfileErr = errors.New("boom")
	_, err = fixture.svc.sessionHasUnauthorizedExactModelDrift(ctx, fixture.current, "dyn-profile")
	require.Error(t, err)
}

type mockUsageProvider struct {
	byID map[string]*agentusage.ProviderUsage
	errs map[string]error
}

func (m *mockUsageProvider) GetUsage(_ context.Context, profileID string) (*agentusage.ProviderUsage, error) {
	if m.errs != nil {
		if err, ok := m.errs[profileID]; ok {
			return nil, err
		}
	}
	if m.byID != nil {
		return m.byID[profileID], nil
	}
	return nil, nil
}

func usageWithRemaining(remainingPct float64) *agentusage.ProviderUsage {
	return &agentusage.ProviderUsage{
		Provider: "test",
		Windows:  []agentusage.UtilizationWindow{{Label: "5h", UtilizationPct: 100 - remainingPct}},
	}
}

// 1. Escalation scenario: two architect sessions on one dynamic profile.
// The older session is the exit initiator from the destination step (step-plan)
// and the newer one is exhausted, so the older author session is picked.
func TestPrepareWorkflowStepSession_EscalationOlderExitAuthorBeatsNewerExhausted(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyReuse, models.WorkflowProfileSessionEndPolicyPark)
	fixture.agentMgr.resolveProfileErr = fmt.Errorf("profile dyn-profile: %w", lifecycle.ErrVirtualProfile)

	const dynamicProfileID = "dyn-profile"
	const olderExecutionProfileID = "p-positive"
	const newerExecutionProfileID = "p-exhausted"

	usage := &mockUsageProvider{
		byID: map[string]*agentusage.ProviderUsage{
			olderExecutionProfileID: usageWithRemaining(40),
			newerExecutionProfileID: usageWithRemaining(0),
		},
	}
	resolver := newWorkflowDynamicProfileResolverWithCandidates(t, dynamicProfileID, []workflowDynamicCandidate{
		{executionProfileID: olderExecutionProfileID, enabled: true},
		{executionProfileID: newerExecutionProfileID, enabled: true},
	})
	resolver.SetUsageProvider(usage)
	fixture.svc.SetProfileExecutionResolver(resolver)

	now := time.Now().UTC()
	olderSession := &models.TaskSession{
		ID:                 "session-arch-older",
		TaskID:             "t1",
		AgentProfileID:     dynamicProfileID,
		ExecutionProfileID: olderExecutionProfileID,
		ExecutorID:         "exec-local",
		ExecutorProfileID:  "ep1",
		TaskEnvironmentID:  "env-1",
		State:              models.TaskSessionStateWaitingForInput,
		StartedAt:          now.Add(-2 * time.Hour),
		UpdatedAt:          now.Add(-2 * time.Hour),
	}
	require.NoError(t, fixture.repo.CreateTaskSession(ctx, olderSession))
	seedExecutorRunning(t, fixture.repo, olderSession.ID, olderSession.TaskID, "execution-arch-older")

	newerSession := &models.TaskSession{
		ID:                 "session-arch-newer",
		TaskID:             "t1",
		AgentProfileID:     dynamicProfileID,
		ExecutionProfileID: newerExecutionProfileID,
		ExecutorID:         "exec-local",
		ExecutorProfileID:  "ep1",
		TaskEnvironmentID:  "env-1",
		State:              models.TaskSessionStateWaitingForInput,
		StartedAt:          now.Add(-1 * time.Hour),
		UpdatedAt:          now.Add(-1 * time.Hour),
	}
	require.NoError(t, fixture.repo.CreateTaskSession(ctx, newerSession))
	seedExecutorRunning(t, fixture.repo, newerSession.ID, newerSession.TaskID, "execution-arch-newer")

	// Set up transition: olderSession exited step-plan to step-implement
	task, err := fixture.repo.GetTask(ctx, "t1")
	require.NoError(t, err)
	task.WorkflowStepID = "step-plan"
	require.NoError(t, fixture.repo.UpdateTask(ctx, task))

	ctxOlderExit := steptelemetry.WithAttribution(ctx, steptelemetry.Attribution{
		Trigger:   steptelemetry.TriggerMCPMove,
		ActorKind: steptelemetry.ActorAgent,
		SessionID: olderSession.ID,
	})
	task.WorkflowStepID = "step-implement"
	require.NoError(t, fixture.repo.UpdateTask(ctxOlderExit, task))

	stepPlan := &wfmodels.WorkflowStep{
		ID:                        "step-plan",
		WorkflowID:                "wf1",
		AgentProfileID:            dynamicProfileID,
		ProfileSessionStartPolicy: models.WorkflowProfileSessionStartPolicyReuse,
	}
	stepImplement := &wfmodels.WorkflowStep{
		ID:                      "step-implement",
		WorkflowID:              "wf1",
		AgentProfileID:          "profile-a",
		ProfileSessionEndPolicy: models.WorkflowProfileSessionEndPolicyPark,
	}
	fixture.stepGetter.steps["step-plan"] = stepPlan
	fixture.stepGetter.steps["step-implement"] = stepImplement

	// Escalation: moving from step-implement back to step-plan
	workSession, switched, err := fixture.svc.prepareWorkflowStepSession(ctx, "t1", fixture.current, stepPlan, stepImplement)
	require.NoError(t, err)
	require.True(t, switched)
	require.NotNil(t, workSession)
	require.Equal(t, olderSession.ID, workSession.ID)
}

// 2. Review -> Implement scenario: the Implement -> Review initiator wins over
// a newer same-profile session because step owner beats freshness.
func TestPrepareWorkflowStepSession_ReviewToImplementCodeAuthorBeatsNewerSession(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyReuse, models.WorkflowProfileSessionEndPolicyPark)

	const coderProfileID = "profile-coder"
	now := time.Now().UTC()

	authorSession := &models.TaskSession{
		ID:                "session-coder-author",
		TaskID:            "t1",
		AgentProfileID:    coderProfileID,
		ExecutorID:        "exec-local",
		ExecutorProfileID: "ep1",
		TaskEnvironmentID: "env-1",
		State:             models.TaskSessionStateWaitingForInput,
		StartedAt:         now.Add(-3 * time.Hour),
		UpdatedAt:         now.Add(-3 * time.Hour),
	}
	require.NoError(t, fixture.repo.CreateTaskSession(ctx, authorSession))
	seedExecutorRunning(t, fixture.repo, authorSession.ID, authorSession.TaskID, "execution-coder-author")

	newerSession := &models.TaskSession{
		ID:                "session-coder-newer",
		TaskID:            "t1",
		AgentProfileID:    coderProfileID,
		ExecutorID:        "exec-local",
		ExecutorProfileID: "ep1",
		TaskEnvironmentID: "env-1",
		State:             models.TaskSessionStateWaitingForInput,
		StartedAt:         now.Add(-1 * time.Hour),
		UpdatedAt:         now.Add(-1 * time.Hour),
	}
	require.NoError(t, fixture.repo.CreateTaskSession(ctx, newerSession))
	seedExecutorRunning(t, fixture.repo, newerSession.ID, newerSession.TaskID, "execution-coder-newer")

	// authorSession exited step-implement to step-review
	task, err := fixture.repo.GetTask(ctx, "t1")
	require.NoError(t, err)
	task.WorkflowStepID = "step-implement"
	require.NoError(t, fixture.repo.UpdateTask(ctx, task))

	ctxAuthorExit := steptelemetry.WithAttribution(ctx, steptelemetry.Attribution{
		Trigger:   steptelemetry.TriggerMCPMove,
		ActorKind: steptelemetry.ActorAgent,
		SessionID: authorSession.ID,
	})
	task.WorkflowStepID = "step-review"
	require.NoError(t, fixture.repo.UpdateTask(ctxAuthorExit, task))

	stepImplement := &wfmodels.WorkflowStep{
		ID:                        "step-implement",
		WorkflowID:                "wf1",
		AgentProfileID:            coderProfileID,
		ProfileSessionStartPolicy: models.WorkflowProfileSessionStartPolicyReuse,
	}
	stepReview := &wfmodels.WorkflowStep{
		ID:                      "step-review",
		WorkflowID:              "wf1",
		AgentProfileID:          "profile-a",
		ProfileSessionEndPolicy: models.WorkflowProfileSessionEndPolicyPark,
	}
	fixture.stepGetter.steps["step-implement"] = stepImplement
	fixture.stepGetter.steps["step-review"] = stepReview

	// Move from review back to implement: author session must win over the newer session
	workSession, switched, err := fixture.svc.prepareWorkflowStepSession(ctx, "t1", fixture.current, stepImplement, stepReview)
	require.NoError(t, err)
	require.True(t, switched)
	require.NotNil(t, workSession)
	require.Equal(t, authorSession.ID, workSession.ID)
}

// 3. All candidates exhausted leading to create-new.
func TestPrepareWorkflowStepSession_AllCandidatesExhaustedFallsThroughToCreateNew(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyReuse, models.WorkflowProfileSessionEndPolicyPark)
	fixture.agentMgr.resolveProfileErr = fmt.Errorf("profile dyn-profile: %w", lifecycle.ErrVirtualProfile)

	const dynamicProfileID = "dyn-profile"
	const exhaustedProfileID = "p-exhausted"
	const availableProfileID = "p-available"

	usage := &mockUsageProvider{
		byID: map[string]*agentusage.ProviderUsage{
			exhaustedProfileID: usageWithRemaining(0),
			availableProfileID: usageWithRemaining(50),
		},
	}
	resolver := newWorkflowDynamicProfileResolverWithCandidates(t, dynamicProfileID, []workflowDynamicCandidate{
		{executionProfileID: exhaustedProfileID, enabled: true},
		{executionProfileID: availableProfileID, enabled: true},
	})
	resolver.SetUsageProvider(usage)
	fixture.svc.SetProfileExecutionResolver(resolver)

	now := time.Now().UTC()
	existingExhausted := &models.TaskSession{
		ID:                 "session-exhausted",
		TaskID:             "t1",
		AgentProfileID:     dynamicProfileID,
		ExecutionProfileID: exhaustedProfileID,
		ExecutorID:         "exec-local",
		ExecutorProfileID:  "ep1",
		TaskEnvironmentID:  "env-1",
		State:              models.TaskSessionStateWaitingForInput,
		StartedAt:          now.Add(-1 * time.Hour),
		UpdatedAt:          now.Add(-1 * time.Hour),
	}
	require.NoError(t, fixture.repo.CreateTaskSession(ctx, existingExhausted))
	seedExecutorRunning(t, fixture.repo, existingExhausted.ID, existingExhausted.TaskID, "execution-exhausted")

	targetStep := &wfmodels.WorkflowStep{
		ID:                        "step-work",
		WorkflowID:                "wf1",
		AgentProfileID:            dynamicProfileID,
		ProfileSessionStartPolicy: models.WorkflowProfileSessionStartPolicyReuse,
	}
	sourceStep := &wfmodels.WorkflowStep{
		ID:                      "step-source",
		WorkflowID:              "wf1",
		AgentProfileID:          "profile-a",
		ProfileSessionEndPolicy: models.WorkflowProfileSessionEndPolicyPark,
	}
	fixture.stepGetter.steps["step-work"] = targetStep
	fixture.stepGetter.steps["step-source"] = sourceStep

	workSession, switched, err := fixture.svc.prepareWorkflowStepSession(ctx, "t1", fixture.current, targetStep, sourceStep)
	require.NoError(t, err)
	require.True(t, switched)
	require.NotNil(t, workSession)
	require.NotEqual(t, existingExhausted.ID, workSession.ID)
	require.Equal(t, dynamicProfileID, workSession.AgentProfileID)
	require.Equal(t, availableProfileID, workSession.ExecutionProfileID)
}

// 4. Pure selector: no-hint legacy behaviour unchanged.
func TestSelectReusableWorkflowSession_NoHintLegacyBehaviourUnchanged(t *testing.T) {
	now := time.Now().UTC()
	sOlder := &models.TaskSession{
		ID:             "s-older",
		AgentProfileID: "profile-p",
		State:          models.TaskSessionStateWaitingForInput,
		UpdatedAt:      now.Add(-2 * time.Hour),
	}
	sNewer := &models.TaskSession{
		ID:             "s-newer",
		AgentProfileID: "profile-p",
		State:          models.TaskSessionStateWaitingForInput,
		UpdatedAt:      now.Add(-1 * time.Hour),
	}
	sTerminal := &models.TaskSession{
		ID:             "s-terminal",
		AgentProfileID: "profile-p",
		State:          models.TaskSessionStateCompleted,
		UpdatedAt:      now,
	}
	sFollowUp := &models.TaskSession{
		ID:             "s-followup",
		AgentProfileID: "profile-p",
		State:          models.TaskSessionStateWaitingForInput,
		UpdatedAt:      now,
		Metadata:       map[string]interface{}{models.SessionMetaKeyCompletionFollowUp: true},
	}
	sOtherProfile := &models.TaskSession{
		ID:             "s-other-profile",
		AgentProfileID: "profile-other",
		State:          models.TaskSessionStateWaitingForInput,
		UpdatedAt:      now,
	}

	sessions := []*models.TaskSession{sOlder, sNewer, sTerminal, sFollowUp, sOtherProfile}
	emptyHint := reuseHint{}

	// 1. Picks the most recently updated nonterminal session
	got := selectReusableWorkflowSession(sessions, "profile-p", "", emptyHint)
	require.NotNil(t, got)
	require.Equal(t, "s-newer", got.ID)

	// 2. Excludes source session (excludeID)
	got = selectReusableWorkflowSession(sessions, "profile-p", "s-newer", emptyHint)
	require.NotNil(t, got)
	require.Equal(t, "s-older", got.ID)

	// 3. Returns nil if all matching sessions are excluded or terminal
	got = selectReusableWorkflowSession([]*models.TaskSession{sTerminal, sFollowUp}, "profile-p", "", emptyHint)
	require.Nil(t, got)

	// 4. Returns nil for unknown profile
	got = selectReusableWorkflowSession(sessions, "profile-unknown", "", emptyHint)
	require.Nil(t, got)

	// 5. Returns nil for empty profile
	got = selectReusableWorkflowSession(sessions, "", "", emptyHint)
	require.Nil(t, got)
}

// 5. Preview and switch parity for the same inputs.
func TestWorkflowMovePreview_PreviewSwitchParity(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyReuse, models.WorkflowProfileSessionEndPolicyPark)

	now := time.Now().UTC()
	targetStep := &wfmodels.WorkflowStep{
		ID:                        "step-target",
		WorkflowID:                "wf1",
		AgentProfileID:            "profile-b",
		ProfileSessionStartPolicy: models.WorkflowProfileSessionStartPolicyReuse,
	}
	sourceStep := &wfmodels.WorkflowStep{
		ID:                      "step-a",
		WorkflowID:              "wf1",
		AgentProfileID:          "profile-a",
		ProfileSessionEndPolicy: models.WorkflowProfileSessionEndPolicyPark,
	}
	fixture.stepGetter.steps[targetStep.ID] = targetStep
	fixture.stepGetter.steps[sourceStep.ID] = sourceStep

	sAuthor := &models.TaskSession{
		ID:                "session-author",
		TaskID:            "t1",
		AgentProfileID:    "profile-b",
		ExecutorID:        "exec-local",
		ExecutorProfileID: "ep1",
		TaskEnvironmentID: "env-1",
		State:             models.TaskSessionStateWaitingForInput,
		StartedAt:         now.Add(-3 * time.Hour),
		UpdatedAt:         now.Add(-3 * time.Hour),
	}
	require.NoError(t, fixture.repo.CreateTaskSession(ctx, sAuthor))
	seedExecutorRunning(t, fixture.repo, sAuthor.ID, sAuthor.TaskID, "execution-author")

	sNewer := &models.TaskSession{
		ID:                "session-newer",
		TaskID:            "t1",
		AgentProfileID:    "profile-b",
		ExecutorID:        "exec-local",
		ExecutorProfileID: "ep1",
		TaskEnvironmentID: "env-1",
		State:             models.TaskSessionStateWaitingForInput,
		StartedAt:         now.Add(-1 * time.Hour),
		UpdatedAt:         now.Add(-1 * time.Hour),
	}
	require.NoError(t, fixture.repo.CreateTaskSession(ctx, sNewer))
	seedExecutorRunning(t, fixture.repo, sNewer.ID, sNewer.TaskID, "execution-newer")

	// sAuthor exited step-target to step-a
	task, err := fixture.repo.GetTask(ctx, "t1")
	require.NoError(t, err)
	task.WorkflowStepID = targetStep.ID
	require.NoError(t, fixture.repo.UpdateTask(ctx, task))

	ctxAuthorExit := steptelemetry.WithAttribution(ctx, steptelemetry.Attribution{
		Trigger:   steptelemetry.TriggerMCPMove,
		ActorKind: steptelemetry.ActorAgent,
		SessionID: sAuthor.ID,
	})
	task.WorkflowStepID = sourceStep.ID
	require.NoError(t, fixture.repo.UpdateTask(ctxAuthorExit, task))

	// 1. Preview
	preview, err := fixture.svc.PreviewWorkflowMove(ctx, WorkflowMovePreviewRequest{
		TaskID:         "t1",
		WorkflowID:     "wf1",
		WorkflowStepID: targetStep.ID,
	})
	require.NoError(t, err)
	require.Equal(t, WorkflowMovePreviewOutcomeReuseOther, preview.Outcome)
	require.NotNil(t, preview.Recipient)

	// 2. Real switch
	workSession, switched, err := fixture.svc.prepareWorkflowStepSession(ctx, "t1", fixture.current, targetStep, sourceStep)
	require.NoError(t, err)
	require.True(t, switched)
	require.NotNil(t, workSession)

	// 3. Parity assertion: both picked sAuthor over sNewer
	require.Equal(t, sAuthor.ID, preview.Recipient.SessionID)
	require.Equal(t, sAuthor.ID, workSession.ID)
	require.Equal(t, preview.Recipient.SessionID, workSession.ID)
}
