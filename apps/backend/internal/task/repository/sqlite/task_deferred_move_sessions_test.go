package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestDeferredMoveRejectsSiblingActivatedBeforeCommit(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-sibling-deferred")
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{
		ID: "workflow-sibling-deferred", WorkspaceID: "workspace-sibling-deferred", Name: "Workflow",
	}))
	seedCASWorkflowStep(t, repo, "workflow-sibling-deferred", "source", 0)
	seedCASWorkflowStep(t, repo, "workflow-sibling-deferred", "target", 1)
	task := &models.Task{
		ID: "task-sibling-deferred", WorkspaceID: "workspace-sibling-deferred",
		WorkflowID: "workflow-sibling-deferred", WorkflowStepID: "source", Title: "Task", WIPAdmitted: true,
	}
	require.NoError(t, repo.CreateTask(ctx, task))
	now := time.Now().UTC()
	owner := &models.TaskSession{
		ID: "owner", TaskID: task.ID, QueueIncarnationID: "owner-incarnation",
		State: models.TaskSessionStateWaitingForInput, StartedAt: now, UpdatedAt: now,
	}
	require.NoError(t, repo.CreateTaskSession(ctx, owner))
	queue, err := messagequeue.NewSQLiteRepository(repo.db, repo.ro)
	require.NoError(t, err)
	move := messagequeue.PendingMove{
		MoveID: "move-sibling-deferred", SessionIncarnationID: owner.QueueIncarnationID,
		TaskID: task.ID, WorkflowID: task.WorkflowID, WorkflowStepID: "target", QueuedAt: now,
	}
	require.NoError(t, queue.SetPendingMove(ctx, owner.ID, &move))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "new-active-sibling", TaskID: task.ID, State: models.TaskSessionStateRunning,
		StartedAt: now, UpdatedAt: now,
	}))

	_, applied, err := repo.UpdateTaskWithWorkflowStepAdmissionForDeferredMove(
		ctx, task, "source", "target", 0,
		messagequeue.PendingMoveRecord{SessionID: owner.ID, Move: move},
	)
	require.Error(t, err)
	require.False(t, applied)
	stored, err := repo.GetTask(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, "source", stored.WorkflowStepID)
	pending, err := queue.GetPendingMove(ctx, owner.ID)
	require.NoError(t, err)
	require.NotNil(t, pending)
}
