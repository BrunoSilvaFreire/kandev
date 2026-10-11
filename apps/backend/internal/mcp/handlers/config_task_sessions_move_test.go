package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleMoveTask_ActiveSiblingOwnsDeferredMove(t *testing.T) {
	svc, repo, wfCtrl, wfRepo := newTestTaskServiceWithWorkflow(t)
	ctx := context.Background()
	seedRunningTask(t, repo, "ws-sibling-move", "wf-sibling-move", "task-sibling-move", "primary", "source")
	require.NoError(t, repo.UpdateTaskSessionState(ctx, "primary", models.TaskSessionStateWaitingForInput, ""))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "sibling", TaskID: "task-sibling-move", State: models.TaskSessionStateRunning,
		StartedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}))
	seedMoveSteps(t, ctx, wfRepo, "wf-sibling-move")

	queue := &pendingMoveRecordingQueuer{}
	h := &Handlers{taskSvc: svc, workflowCtrl: wfCtrl, messageQueue: queue, logger: testLogger(t).WithFields()}
	resp, err := h.handleMoveTask(ctx, makeWSMessage(t, ws.ActionMCPMoveTask, map[string]interface{}{
		"task_id": "task-sibling-move", "workflow_id": "wf-sibling-move", "workflow_step_id": "target",
	}))
	require.NoError(t, err)
	assert.Equal(t, ws.MessageTypeResponse, resp.Type)
	require.Len(t, queue.pendingMoves, 1)
	assert.Equal(t, "sibling", queue.pendingSessionID)
}

func TestHandleMoveTask_MultipleActiveSessionsConflict(t *testing.T) {
	svc, repo, wfCtrl, wfRepo := newTestTaskServiceWithWorkflow(t)
	ctx := context.Background()
	seedRunningTask(t, repo, "ws-multi-move", "wf-multi-move", "task-multi-move", "primary", "source")
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "sibling", TaskID: "task-multi-move", State: models.TaskSessionStateRunning,
		StartedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}))
	seedMoveSteps(t, ctx, wfRepo, "wf-multi-move")

	queue := &pendingMoveRecordingQueuer{}
	h := &Handlers{taskSvc: svc, workflowCtrl: wfCtrl, messageQueue: queue, logger: testLogger(t).WithFields()}
	resp, err := h.handleMoveTask(ctx, makeWSMessage(t, ws.ActionMCPMoveTask, map[string]interface{}{
		"task_id": "task-multi-move", "workflow_id": "wf-multi-move", "workflow_step_id": "target",
	}))
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeConflict)
	assert.Empty(t, queue.pendingMoves)
}

func seedMoveSteps(t *testing.T, ctx context.Context, repo interface {
	CreateStep(context.Context, *workflowmodels.WorkflowStep) error
}, workflowID string) {
	t.Helper()
	now := time.Now().UTC()
	require.NoError(t, repo.CreateStep(ctx, &workflowmodels.WorkflowStep{
		ID: "source", WorkflowID: workflowID, Name: "Source", Position: 0, CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateStep(ctx, &workflowmodels.WorkflowStep{
		ID: "target", WorkflowID: workflowID, Name: "Review", Position: 1, CreatedAt: now, UpdatedAt: now,
	}))
}
