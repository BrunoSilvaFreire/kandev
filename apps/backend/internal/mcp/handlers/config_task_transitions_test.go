package handlers

import (
	"context"
	"testing"
	"time"

	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowmove "github.com/kandev/kandev/internal/workflow/move"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedTransitionSteps creates a source step at position 2 carrying a named
// transition and a target step at position 3 in the same workflow.
func seedTransitionSteps(t *testing.T, wfRepo interface {
	CreateStep(context.Context, *workflowmodels.WorkflowStep) error
}, wfID string) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	require.NoError(t, wfRepo.CreateStep(ctx, &workflowmodels.WorkflowStep{
		ID: "arch", WorkflowID: wfID, Name: "Architect", Position: 2, CreatedAt: now, UpdatedAt: now,
		Events: workflowmodels.StepEvents{Transitions: []workflowmodels.StepTransition{{
			Name:           "handoff",
			Direction:      workflowmodels.TransitionDirectionForward,
			ToStepID:       "impl",
			Instructions:   "ARCHITECT HANDOFF",
			SkipStepPrompt: true,
		}}},
	}))
	require.NoError(t, wfRepo.CreateStep(ctx, &workflowmodels.WorkflowStep{
		ID: "impl", WorkflowID: wfID, Name: "Implement", Position: 3, CreatedAt: now, UpdatedAt: now,
	}))
}

func TestHandleMoveTask_TransitionDefersToResolvedTarget(t *testing.T) {
	svc, repo, wfCtrl, wfRepo := newTestTaskServiceWithWorkflow(t)
	ctx := context.Background()
	seedRunningTask(t, repo, "ws-tr1", "wf-tr1", "task-tr1", "sess-tr1", "arch")
	seedTransitionSteps(t, wfRepo, "wf-tr1")

	queue := &pendingMoveRecordingQueuer{}
	h := &Handlers{taskSvc: svc, workflowCtrl: wfCtrl, messageQueue: queue, logger: testLogger(t).WithFields()}

	msg := makeWSMessage(t, ws.ActionMCPMoveTask, map[string]interface{}{
		"task_id":    "task-tr1",
		"transition": "handoff",
		"entry_options": map[string]interface{}{
			"instructions": "resume from here",
		},
	})
	resp, err := h.handleMoveTask(ctx, msg)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Len(t, queue.pendingMoves, 1)
	move := queue.pendingMoves[0]
	assert.Equal(t, "wf-tr1", move.WorkflowID)
	assert.Equal(t, "impl", move.WorkflowStepID)
	require.NotNil(t, move.EntryOptions)
	assert.Contains(t, move.EntryOptions.Instructions, "ARCHITECT HANDOFF")
	assert.Contains(t, move.EntryOptions.Instructions, "resume from here")
	assert.True(t, move.EntryOptions.SkipStepPrompt, "transition skip_step_prompt must be OR-ed in")
}

func TestHandleMoveTask_UnknownTransitionIsRejected(t *testing.T) {
	svc, repo, wfCtrl, wfRepo := newTestTaskServiceWithWorkflow(t)
	ctx := context.Background()
	seedRunningTask(t, repo, "ws-tr2", "wf-tr2", "task-tr2", "sess-tr2", "arch")
	seedTransitionSteps(t, wfRepo, "wf-tr2")

	queue := &pendingMoveRecordingQueuer{}
	h := &Handlers{taskSvc: svc, workflowCtrl: wfCtrl, messageQueue: queue, logger: testLogger(t).WithFields()}
	msg := makeWSMessage(t, ws.ActionMCPMoveTask, map[string]interface{}{
		"task_id": "task-tr2", "transition": "nope",
	})
	resp, err := h.handleMoveTask(ctx, msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
	assert.Empty(t, queue.pendingMoves)
}

func TestHandleMoveTask_TransitionAndStepIDAreMutuallyExclusive(t *testing.T) {
	h := &Handlers{}
	msg := makeWSMessage(t, ws.ActionMCPMoveTask, map[string]interface{}{
		"task_id":          "task-1",
		"transition":       "handoff",
		"workflow_step_id": "impl",
	})
	resp, err := h.handleMoveTask(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}

func TestHandleMoveTask_TransitionWithoutResolverFailsClosed(t *testing.T) {
	h := &Handlers{}
	msg := makeWSMessage(t, ws.ActionMCPMoveTask, map[string]interface{}{
		"task_id":    "task-1",
		"transition": "handoff",
	})
	resp, err := h.handleMoveTask(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}

func TestMergeTransitionEntryOptions(t *testing.T) {
	tr := &workflowmodels.StepTransition{Instructions: "step intent", SkipStepPrompt: true}
	merged := mergeTransitionEntryOptions(tr, nil)
	require.NotNil(t, merged)
	assert.Equal(t, "step intent", merged.Instructions)
	assert.True(t, merged.SkipStepPrompt)

	merged = mergeTransitionEntryOptions(tr, &workflowmove.EntryOptions{Instructions: "caller", ResetContext: true})
	require.NotNil(t, merged)
	assert.Contains(t, merged.Instructions, "step intent")
	assert.Contains(t, merged.Instructions, "caller")
	assert.True(t, merged.ResetContext)

	empty := mergeTransitionEntryOptions(&workflowmodels.StepTransition{}, nil)
	assert.Nil(t, empty)
}
