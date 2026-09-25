package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/stretchr/testify/require"
)

// denyWorkflowRoutePromotionRepo fails the atomic route promotion while
// delegating everything else to the real repository. It deterministically
// reproduces the race where a validated reusable candidate stops being
// promotable between the decision and the promotion.
type denyWorkflowRoutePromotionRepo struct {
	*sqliterepo.Repository
	denySessionID string
}

func (r denyWorkflowRoutePromotionRepo) SetSessionPrimaryWithWorkflowSessionRouteIfNonterminal(
	ctx context.Context, sessionID string, route models.WorkflowSessionRoute,
) (bool, error) {
	if sessionID == r.denySessionID {
		return false, nil
	}
	return r.Repository.SetSessionPrimaryWithWorkflowSessionRouteIfNonterminal(ctx, sessionID, route)
}

func routeLedgerRows(t *testing.T, repo *sqliterepo.Repository, taskID string) []*models.TaskSessionRoute {
	t.Helper()
	rows, err := repo.ListTaskSessionRoutes(context.Background(), taskID)
	require.NoError(t, err)
	return rows
}

func requireSingleRoute(
	t *testing.T,
	repo *sqliterepo.Repository,
	taskID string,
	outcome models.RoutingOutcome,
	reason models.RoutingReason,
	destination *models.TaskSession,
) *models.TaskSessionRoute {
	t.Helper()
	rows := routeLedgerRows(t, repo, taskID)
	require.Len(t, rows, 1, "exactly one route row per entry")
	row := rows[0]
	require.Equal(t, outcome, row.Outcome)
	require.Equal(t, reason, row.Reason)
	require.NotNil(t, destination)
	require.NotNil(t, row.DestinationSessionID)
	require.Equal(t, destination.ID, *row.DestinationSessionID, "ledger destination must equal the promoted session")
	require.Equal(t, taskID, row.TaskID)
	return row
}

func newProfileSwitchStep(id string, position int, policy models.WorkflowProfileSessionStartPolicy) *wfmodels.WorkflowStep {
	return &wfmodels.WorkflowStep{
		ID: id, WorkflowID: "wf1", Position: position,
		ProfileSessionStartPolicy: policy,
		ProfileSessionEndPolicy:   models.WorkflowProfileSessionEndPolicyPark,
	}
}

func TestPrepareWorkflowStepSession_RouteLedger_ReusedCurrentSession(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyReuse, models.WorkflowProfileSessionEndPolicyPark)
	fixture.stepGetter.workflowAgentProfileID = "profile-a"

	step := newProfileSwitchStep("step-a", 0, models.WorkflowProfileSessionStartPolicyReuse)
	selected, switched, err := fixture.svc.prepareWorkflowStepSession(ctx, "t1", fixture.current, step, nil)
	require.NoError(t, err)
	require.False(t, switched)
	require.Equal(t, fixture.current.ID, selected.ID)

	requireSingleRoute(t, fixture.repo, "t1", models.RoutingOutcomeReused, models.RoutingReasonReusedCurrentSession, selected)
}

func TestPrepareWorkflowStepSession_RouteLedger_NoReusableCandidate(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyReuse, models.WorkflowProfileSessionEndPolicyPark)

	selected, switched, err := fixture.svc.prepareWorkflowStepSession(ctx, "t1", fixture.current,
		newProfileSwitchStep("step-b", 1, models.WorkflowProfileSessionStartPolicyReuse),
		newProfileSwitchStep("step-a", 0, models.WorkflowProfileSessionStartPolicyReuse),
	)
	require.NoError(t, err)
	require.True(t, switched)

	requireSingleRoute(t, fixture.repo, "t1", models.RoutingOutcomeCreated, models.RoutingReasonNoReusableCandidate, selected)
}

func TestPrepareWorkflowStepSession_RouteLedger_ReusedExisting(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyReuse, models.WorkflowProfileSessionEndPolicyPark)
	candidate := &models.TaskSession{
		ID: "session-b", TaskID: "t1", AgentProfileID: "profile-b",
		ExecutorID: "exec-local", ExecutorProfileID: "ep1", TaskEnvironmentID: "env-1",
		State: models.TaskSessionStateWaitingForInput, StartedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	require.NoError(t, fixture.repo.CreateTaskSession(ctx, candidate))

	selected, switched, err := fixture.svc.prepareWorkflowStepSession(ctx, "t1", fixture.current,
		newProfileSwitchStep("step-b", 1, models.WorkflowProfileSessionStartPolicyReuse),
		newProfileSwitchStep("step-a", 0, models.WorkflowProfileSessionStartPolicyReuse),
	)
	require.NoError(t, err)
	require.True(t, switched)
	require.Equal(t, candidate.ID, selected.ID)

	requireSingleRoute(t, fixture.repo, "t1", models.RoutingOutcomeReused, models.RoutingReasonReusedExisting, selected)
}

func TestPrepareWorkflowStepSession_RouteLedger_ForcedNewPolicy(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyNew, models.WorkflowProfileSessionEndPolicyPark)

	selected, switched, err := fixture.svc.prepareWorkflowStepSession(ctx, "t1", fixture.current,
		newProfileSwitchStep("step-b", 1, models.WorkflowProfileSessionStartPolicyNew),
		newProfileSwitchStep("step-a", 0, models.WorkflowProfileSessionStartPolicyNew),
	)
	require.NoError(t, err)
	require.True(t, switched)

	row := requireSingleRoute(t, fixture.repo, "t1", models.RoutingOutcomeCreated, models.RoutingReasonForcedNewPolicy, selected)
	require.Equal(t, string(models.WorkflowProfileSessionStartPolicyNew), row.StartPolicy)
}

func TestPrepareWorkflowStepSession_RouteLedger_ExactModelReplaceInPlace(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyReuse, models.WorkflowProfileSessionEndPolicyPark)
	fixture.agentMgr.resolveProfileInfo = &executor.AgentProfileInfo{Model: "gpt-5.6-terra", RequireExactModel: true}
	fixture.current.AgentProfileID = "terra-work-profile"
	fixture.current.Metadata = map[string]interface{}{
		models.SessionMetaKeyRuntimeConfig: models.SessionRuntimeConfig{Model: "gpt-5.6-luna"},
	}
	require.NoError(t, fixture.repo.UpdateTaskSession(ctx, fixture.current))

	selected, switched, err := fixture.svc.prepareWorkflowStepSession(ctx, "t1", fixture.current,
		&wfmodels.WorkflowStep{ID: "step-work", WorkflowID: "wf1", Position: 1,
			AgentProfileID:            "terra-work-profile",
			ProfileSessionStartPolicy: models.WorkflowProfileSessionStartPolicyReuse},
		&wfmodels.WorkflowStep{ID: "step-qa", WorkflowID: "wf1", AgentProfileID: "terra-work-profile",
			ProfileSessionEndPolicy: models.WorkflowProfileSessionEndPolicyPark},
	)
	require.NoError(t, err)
	require.True(t, switched)
	require.NotEqual(t, fixture.current.ID, selected.ID)

	requireSingleRoute(t, fixture.repo, "t1", models.RoutingOutcomeCreated, models.RoutingReasonExactModelIncompatibility, selected)
}

func TestPrepareWorkflowStepSession_RouteLedger_ExactModelCandidateIncompatibility(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyReuse, models.WorkflowProfileSessionEndPolicyPark)
	fixture.agentMgr.resolveProfileInfo = &executor.AgentProfileInfo{Model: "gpt-5.6-terra", RequireExactModel: true}
	candidate := &models.TaskSession{
		ID: "session-b", TaskID: "t1", AgentProfileID: "profile-b",
		ExecutorID: "exec-local", ExecutorProfileID: "ep1", TaskEnvironmentID: "env-1",
		State: models.TaskSessionStateWaitingForInput, StartedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
		Metadata: map[string]interface{}{
			models.SessionMetaKeyRuntimeConfig: models.SessionRuntimeConfig{Model: "gpt-5.6-luna"},
		},
	}
	require.NoError(t, fixture.repo.CreateTaskSession(ctx, candidate))

	selected, switched, err := fixture.svc.prepareWorkflowStepSession(ctx, "t1", fixture.current,
		newProfileSwitchStep("step-b", 1, models.WorkflowProfileSessionStartPolicyReuse),
		newProfileSwitchStep("step-a", 0, models.WorkflowProfileSessionStartPolicyReuse),
	)
	require.NoError(t, err)
	require.True(t, switched)
	require.NotEqual(t, candidate.ID, selected.ID)

	requireSingleRoute(t, fixture.repo, "t1", models.RoutingOutcomeCreated, models.RoutingReasonExactModelIncompatibility, selected)
}

func TestPrepareWorkflowStepSession_RouteLedger_SelectedCandidateTerminal(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyReuse, models.WorkflowProfileSessionEndPolicyPark)
	candidate := &models.TaskSession{
		ID: "session-b", TaskID: "t1", AgentProfileID: "profile-b",
		ExecutorID: "exec-local", ExecutorProfileID: "ep1", TaskEnvironmentID: "env-1",
		State: models.TaskSessionStateWaitingForInput, StartedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	require.NoError(t, fixture.repo.CreateTaskSession(ctx, candidate))
	fixture.svc.repo = denyWorkflowRoutePromotionRepo{Repository: fixture.repo, denySessionID: candidate.ID}

	selected, switched, err := fixture.svc.prepareWorkflowStepSession(ctx, "t1", fixture.current,
		newProfileSwitchStep("step-b", 1, models.WorkflowProfileSessionStartPolicyReuse),
		newProfileSwitchStep("step-a", 0, models.WorkflowProfileSessionStartPolicyReuse),
	)
	require.NoError(t, err)
	require.True(t, switched)
	require.NotEqual(t, candidate.ID, selected.ID)

	requireSingleRoute(t, fixture.repo, "t1", models.RoutingOutcomeCreated, models.RoutingReasonSelectedCandidateTerminal, selected)
}

// explicitWorkflowTargetFixture seeds a source step bound to a nonterminal
// session so the destination's explicit session target resolves to it.
func explicitWorkflowTargetFixture(t *testing.T, startPolicy models.WorkflowProfileSessionStartPolicy) (*profileSwitchFixture, *wfmodels.WorkflowStep) {
	t.Helper()
	fixture := newProfileSwitchFixture(t, startPolicy, models.WorkflowProfileSessionEndPolicyPark)
	source := &wfmodels.WorkflowStep{ID: "step-a", WorkflowID: "wf1", Position: 0, AgentProfileID: "profile-a"}
	fixture.stepGetter.steps[source.ID] = source
	target := &wfmodels.WorkflowStep{
		ID: "step-review", WorkflowID: "wf1", Position: 1,
		SessionTarget:             &wfmodels.WorkflowSessionTarget{Kind: wfmodels.WorkflowSessionTargetStep, StepID: source.ID},
		ProfileSessionStartPolicy: startPolicy,
	}
	return fixture, target
}

func TestPrepareWorkflowStepSession_RouteLedger_ExplicitTargetResolved(t *testing.T) {
	ctx := context.Background()
	fixture, target := explicitWorkflowTargetFixture(t, models.WorkflowProfileSessionStartPolicyReuse)
	bound := &models.TaskSession{
		ID: "session-bound", TaskID: "t1", AgentProfileID: "profile-a",
		ExecutorID: "exec-local", ExecutorProfileID: "ep1", TaskEnvironmentID: "env-1",
		State: models.TaskSessionStateWaitingForInput, StartedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	require.NoError(t, fixture.repo.CreateTaskSession(ctx, bound))
	require.NoError(t, fixture.svc.recordWorkflowSourceBinding(ctx, "t1",
		&wfmodels.WorkflowStep{ID: "step-a", WorkflowID: "wf1", AgentProfileID: "profile-a"}, bound))

	selected, switched, err := fixture.svc.prepareWorkflowStepSession(ctx, "t1", fixture.current, target,
		newProfileSwitchStep("step-a", 0, models.WorkflowProfileSessionStartPolicyReuse), int64(51),
	)
	require.NoError(t, err)
	require.True(t, switched)
	require.Equal(t, bound.ID, selected.ID)

	requireSingleRoute(t, fixture.repo, "t1", models.RoutingOutcomeReused, models.RoutingReasonExplicitTarget, selected)
}

func TestPrepareWorkflowStepSession_RouteLedger_ExplicitTargetRecordedRoute(t *testing.T) {
	ctx := context.Background()
	fixture, target := explicitWorkflowTargetFixture(t, models.WorkflowProfileSessionStartPolicyReuse)
	recorded := &models.TaskSession{
		ID: "session-recorded", TaskID: "t1", AgentProfileID: "profile-a",
		ExecutorID: "exec-local", ExecutorProfileID: "ep1", TaskEnvironmentID: "env-1",
		State: models.TaskSessionStateWaitingForInput, StartedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	require.NoError(t, fixture.repo.CreateTaskSession(ctx, recorded))

	entryIDs := []int64{52}
	entryIdentity := fixture.svc.workflowEntryIdentity(ctx, "t1", entryIDs...)
	operationID := workflowSessionRouteID("t1", target.ID, entryIdentity, target.SessionTarget,
		models.WorkflowProfileSessionStartPolicyReuse)
	require.NoError(t, fixture.repo.SetTaskMetadataKey(ctx, "t1", models.MetaKeyWorkflowSessionRoute, models.WorkflowSessionRoute{
		OperationID:       operationID,
		DestinationStepID: target.ID,
		EntryIdentity:     entryIdentity,
		TargetKind:        string(target.SessionTarget.Kind),
		TargetStepID:      target.SessionTarget.StepID,
		AgentProfileID:    "profile-a",
		DestinationID:     recorded.ID,
		Phase:             workflowSessionRouteCommitted,
	}))

	selected, switched, err := fixture.svc.prepareWorkflowStepSession(ctx, "t1", fixture.current, target,
		newProfileSwitchStep("step-a", 0, models.WorkflowProfileSessionStartPolicyReuse), entryIDs...,
	)
	require.NoError(t, err)
	require.True(t, switched)
	require.Equal(t, recorded.ID, selected.ID)

	requireSingleRoute(t, fixture.repo, "t1", models.RoutingOutcomeReused, models.RoutingReasonExplicitTarget, selected)
}

func TestPrepareWorkflowStepSession_RouteLedger_ExplicitTargetCreated(t *testing.T) {
	ctx := context.Background()
	fixture, target := explicitWorkflowTargetFixture(t, models.WorkflowProfileSessionStartPolicyNew)

	selected, switched, err := fixture.svc.prepareWorkflowStepSession(ctx, "t1", fixture.current, target,
		newProfileSwitchStep("step-a", 0, models.WorkflowProfileSessionStartPolicyNew),
	)
	require.NoError(t, err)
	require.True(t, switched)

	requireSingleRoute(t, fixture.repo, "t1", models.RoutingOutcomeCreated, models.RoutingReasonExplicitTarget, selected)
}

func TestPrepareWorkflowStepSession_SessionCreatedWithDestinationStepProvenance(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyReuse, models.WorkflowProfileSessionEndPolicyPark)
	// A committed move updates the task's current step before on_enter runs, so
	// the created session's destination step is the task's current step.
	task, err := fixture.repo.GetTask(ctx, "t1")
	require.NoError(t, err)
	task.WorkflowStepID = "step-b"
	require.NoError(t, fixture.repo.UpdateTask(ctx, task))

	selected, switched, err := fixture.svc.prepareWorkflowStepSession(ctx, "t1", fixture.current,
		newProfileSwitchStep("step-b", 1, models.WorkflowProfileSessionStartPolicyReuse),
		newProfileSwitchStep("step-a", 0, models.WorkflowProfileSessionStartPolicyReuse),
	)
	require.NoError(t, err)
	require.True(t, switched)
	require.Equal(t, "step-b", selected.WorkflowStepIDAtCreation,
		"a session created during a move records the destination step, not the source")
}

func TestPrepareWorkflowStepSession_RouteLedger_IdempotentReplay(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyReuse, models.WorkflowProfileSessionEndPolicyPark)
	fixture.stepGetter.workflowAgentProfileID = "profile-a"
	step := newProfileSwitchStep("step-a", 0, models.WorkflowProfileSessionStartPolicyReuse)
	entryIDs := []int64{61}

	first, _, err := fixture.svc.prepareWorkflowStepSession(ctx, "t1", fixture.current, step, nil, entryIDs...)
	require.NoError(t, err)
	second, _, err := fixture.svc.prepareWorkflowStepSession(ctx, "t1", fixture.current, step, nil, entryIDs...)
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID)

	requireSingleRoute(t, fixture.repo, "t1", models.RoutingOutcomeReused, models.RoutingReasonReusedCurrentSession, first)
}
