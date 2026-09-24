package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/entryroute"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

// TestRestoreTaskMessageRollbackFreezesTaggedEntryRoute proves the rollback
// restore commits a new tagged entry with a freshly frozen route. Restoring a
// tagged step creates a new transition identity, so the pre-failure route is
// stale; without a fresh choice the restored entry could not launch.
func TestRestoreTaskMessageRollbackFreezesTaggedEntryRoute(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	setupTestTask(t, repo)
	sessionID := setupTestSession(t, repo)
	require.NoError(t, repo.UpdateTaskSessionState(
		ctx, sessionID, models.TaskSessionStateRunning, "",
	))
	svc.SetWorkflowStepGetter(&fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"restored-step": {ID: "restored-step", WorkflowID: "wf-123", AllowedTags: []string{"review"}},
	}})
	selector := &recordingEntrySelector{profile: "profile-frozen"}
	svc.SetWorkflowEntryProfileSelector(selector)

	_, updated, err := svc.RestoreTaskMessageRollback(
		ctx, "task-123", sessionID, models.TaskSessionStateRunning, v1.TaskStateReview, "restored-step",
	)
	require.NoError(t, err)
	require.True(t, updated)
	require.Equal(t, 1, selector.calls)

	restored, err := repo.GetTask(ctx, "task-123")
	require.NoError(t, err)
	require.Equal(t, "restored-step", restored.WorkflowStepID)
	route, ok := models.LoadWorkflowSessionRoute(restored.Metadata)
	require.True(t, ok, "tagged rollback must persist a frozen route")
	require.Equal(t, "profile-frozen", route.AgentProfileID)
	require.Equal(t, "restored-step", route.DestinationStepID)
	require.Equal(t, entryroute.TargetKindProfile, route.TargetKind)
	require.NotEmpty(t, route.EntryIdentity)
}

// TestRestoreTaskMessageRollbackFailsClosedWhenTaggedSelectionUnavailable
// proves a tagged restore does not commit without a frozen choice when no
// selector is wired.
func TestRestoreTaskMessageRollbackFailsClosedWhenTaggedSelectionUnavailable(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	setupTestTask(t, repo)
	sessionID := setupTestSession(t, repo)
	require.NoError(t, repo.UpdateTaskSessionState(
		ctx, sessionID, models.TaskSessionStateRunning, "",
	))
	svc.SetWorkflowStepGetter(&fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"restored-step": {ID: "restored-step", WorkflowID: "wf-123", AllowedTags: []string{"review"}},
	}})

	_, updated, err := svc.RestoreTaskMessageRollback(
		ctx, "task-123", sessionID, models.TaskSessionStateRunning, v1.TaskStateReview, "restored-step",
	)
	require.ErrorIs(t, err, entryroute.ErrSelectorUnavailable)
	require.False(t, updated)

	persisted, getErr := repo.GetTask(ctx, "task-123")
	require.NoError(t, getErr)
	require.Equal(t, "step-123", persisted.WorkflowStepID, "a failed restore must not move the task")
}
