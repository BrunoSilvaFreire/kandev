package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/stretchr/testify/require"
)

func TestPrepareWorkflowStepSession_DesignationOverridesSameProfileKeepCurrent(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyReuse, models.WorkflowProfileSessionEndPolicyPark)

	// Current session already has the destination step's profile, so without a
	// designation profile-only routing would keep it in place.
	fixture.current.AgentProfileID = "profile-b"
	require.NoError(t, fixture.repo.UpdateTaskSession(ctx, fixture.current))

	designated := &models.TaskSession{
		ID: "session-designated", TaskID: "t1", AgentProfileID: "profile-b",
		ExecutorID: "exec-local", ExecutorProfileID: "ep1", TaskEnvironmentID: "env-1",
		State: models.TaskSessionStateWaitingForInput, StartedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	require.NoError(t, fixture.repo.CreateTaskSession(ctx, designated))
	seedExecutorRunning(t, fixture.repo, designated.ID, designated.TaskID, "execution-designated")
	require.NoError(t, fixture.repo.SetTaskMetadataKey(
		ctx, "t1", models.MetaKeyStepPrimarySessions, map[string]string{"step-work": designated.ID},
	))

	targetStep := &wfmodels.WorkflowStep{
		ID: "step-work", WorkflowID: "wf1", AgentProfileID: "profile-b",
		ProfileSessionStartPolicy: models.WorkflowProfileSessionStartPolicyReuse,
	}
	sourceStep := &wfmodels.WorkflowStep{ID: "step-source", WorkflowID: "wf1", AgentProfileID: "profile-b"}

	workSession, switched, err := fixture.svc.prepareWorkflowStepSession(ctx, "t1", fixture.current, targetStep, sourceStep)
	require.NoError(t, err)
	require.True(t, switched)
	require.NotNil(t, workSession)
	require.Equal(t, designated.ID, workSession.ID)
}

func TestSelectReusableWorkflowSession_DesignatedBeatsFreshnessAndProfile(t *testing.T) {
	now := time.Now().UTC()
	designated := &models.TaskSession{
		ID:             "sess-designated",
		AgentProfileID: "profile-designated",
		State:          models.TaskSessionStateWaitingForInput,
		UpdatedAt:      now.Add(-4 * time.Hour),
	}
	newer := &models.TaskSession{
		ID:             "sess-newer",
		AgentProfileID: "profile-step",
		State:          models.TaskSessionStateWaitingForInput,
		UpdatedAt:      now,
	}
	sessions := []*models.TaskSession{newer, designated}

	got := selectReusableWorkflowSession(sessions, "profile-step", "", reuseHint{
		DesignatedSessionID: designated.ID,
	})
	if got == nil || got.ID != designated.ID {
		t.Fatalf("selected %#v, want the designated session regardless of profile or freshness", got)
	}
}

func TestSelectReusableWorkflowSession_DesignatedBeatsExitAuthor(t *testing.T) {
	now := time.Now().UTC()
	designated := &models.TaskSession{
		ID:             "sess-designated",
		AgentProfileID: "profile-step",
		State:          models.TaskSessionStateWaitingForInput,
		UpdatedAt:      now.Add(-2 * time.Hour),
	}
	author := &models.TaskSession{
		ID:             "sess-author",
		AgentProfileID: "profile-step",
		State:          models.TaskSessionStateWaitingForInput,
		UpdatedAt:      now,
	}
	got := selectReusableWorkflowSession([]*models.TaskSession{author, designated}, "profile-step", "", reuseHint{
		DesignatedSessionID: designated.ID,
		StepOwnerSessionID:  author.ID,
	})
	if got == nil || got.ID != designated.ID {
		t.Fatalf("selected %#v, want the designation to outrank the step exit author", got)
	}
}

func TestSelectReusableWorkflowSession_DesignatedTerminalIgnored(t *testing.T) {
	now := time.Now().UTC()
	designated := &models.TaskSession{
		ID:             "sess-designated",
		AgentProfileID: "profile-step",
		State:          models.TaskSessionStateCompleted,
		UpdatedAt:      now,
	}
	fallback := &models.TaskSession{
		ID:             "sess-fallback",
		AgentProfileID: "profile-step",
		State:          models.TaskSessionStateWaitingForInput,
		UpdatedAt:      now.Add(-time.Hour),
	}
	got := selectReusableWorkflowSession([]*models.TaskSession{designated, fallback}, "profile-step", "", reuseHint{
		DesignatedSessionID: designated.ID,
	})
	if got == nil || got.ID != fallback.ID {
		t.Fatalf("selected %#v, want the profile match when the designation is terminal", got)
	}
}

func TestSelectReusableWorkflowSession_DesignatedExhaustedExcluded(t *testing.T) {
	now := time.Now().UTC()
	designated := &models.TaskSession{
		ID:             "sess-designated",
		AgentProfileID: "profile-step",
		State:          models.TaskSessionStateWaitingForInput,
		UpdatedAt:      now,
	}
	fallback := &models.TaskSession{
		ID:             "sess-fallback",
		AgentProfileID: "profile-step",
		State:          models.TaskSessionStateWaitingForInput,
		UpdatedAt:      now.Add(-time.Hour),
	}
	got := selectReusableWorkflowSession([]*models.TaskSession{designated, fallback}, "profile-step", "", reuseHint{
		DesignatedSessionID: designated.ID,
		Exhausted:           func(session *models.TaskSession) bool { return session.ID == designated.ID },
	})
	if got == nil || got.ID != fallback.ID {
		t.Fatalf("selected %#v, want an exhausted designation to be excluded", got)
	}
}

func TestBuildWorkflowMovePreview_DesignatedSessionWins(t *testing.T) {
	now := time.Now().UTC()
	current := &models.TaskSession{
		ID:             "sess-current",
		TaskID:         "task-1",
		AgentProfileID: "profile-current",
		State:          models.TaskSessionStateWaitingForInput,
	}
	designated := &models.TaskSession{
		ID:             "sess-designated",
		TaskID:         "task-1",
		AgentProfileID: "profile-other",
		State:          models.TaskSessionStateWaitingForInput,
		UpdatedAt:      now.Add(-3 * time.Hour),
	}
	newer := &models.TaskSession{
		ID:             "sess-newer",
		TaskID:         "task-1",
		AgentProfileID: "profile-step",
		State:          models.TaskSessionStateWaitingForInput,
		UpdatedAt:      now,
	}

	preview := buildWorkflowMovePreview(workflowMovePreviewInput{
		TaskID:          "task-1",
		SourceSession:   current,
		Sessions:        []*models.TaskSession{current, designated, newer},
		Destination:     &wfmodels.WorkflowStep{ID: "step-implement", Name: "Implement"},
		Source:          &wfmodels.WorkflowStep{ID: "step-analysis", Name: "Analysis"},
		TargetProfileID: "profile-step",
		StartPolicy:     models.WorkflowProfileSessionStartPolicyReuse,
		SourceEndPolicy: models.WorkflowProfileSessionEndPolicyPark,
		ReuseHint:       reuseHint{DesignatedSessionID: designated.ID},
	})

	if preview.Outcome != WorkflowMovePreviewOutcomeReuseOther {
		t.Fatalf("outcome = %q, want reuse_other", preview.Outcome)
	}
	if preview.Recipient == nil || preview.Recipient.SessionID != designated.ID {
		t.Fatalf("recipient = %#v, want the designated session", preview.Recipient)
	}
}

func TestBuildWorkflowMovePreview_DesignatedTerminalFallsThrough(t *testing.T) {
	now := time.Now().UTC()
	current := &models.TaskSession{
		ID:             "sess-current",
		TaskID:         "task-1",
		AgentProfileID: "profile-current",
		State:          models.TaskSessionStateWaitingForInput,
	}
	designated := &models.TaskSession{
		ID:             "sess-designated",
		TaskID:         "task-1",
		AgentProfileID: "profile-step",
		State:          models.TaskSessionStateCompleted,
		UpdatedAt:      now,
	}
	newer := &models.TaskSession{
		ID:             "sess-newer",
		TaskID:         "task-1",
		AgentProfileID: "profile-step",
		State:          models.TaskSessionStateWaitingForInput,
		UpdatedAt:      now.Add(-time.Hour),
	}

	preview := buildWorkflowMovePreview(workflowMovePreviewInput{
		TaskID:          "task-1",
		SourceSession:   current,
		Sessions:        []*models.TaskSession{current, designated, newer},
		Destination:     &wfmodels.WorkflowStep{ID: "step-implement", Name: "Implement"},
		Source:          &wfmodels.WorkflowStep{ID: "step-analysis", Name: "Analysis"},
		TargetProfileID: "profile-step",
		StartPolicy:     models.WorkflowProfileSessionStartPolicyReuse,
		SourceEndPolicy: models.WorkflowProfileSessionEndPolicyPark,
		ReuseHint:       reuseHint{DesignatedSessionID: designated.ID},
	})

	if preview.Outcome != WorkflowMovePreviewOutcomeReuseOther {
		t.Fatalf("outcome = %q, want reuse_other", preview.Outcome)
	}
	if preview.Recipient == nil || preview.Recipient.SessionID != newer.ID {
		t.Fatalf("recipient = %#v, want the profile-matching fallback", preview.Recipient)
	}
}
