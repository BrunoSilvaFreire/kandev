package handlers

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleGetTaskCompletionGate(t *testing.T) {
	svc, repo, wfCtrl, _ := newTestTaskServiceWithWorkflow(t)
	ctx := context.Background()
	seedRunningTask(t, repo, "ws-cg1", "wf-cg1", "task-cg1", "sess-cg1", "step-1")

	h := &Handlers{taskSvc: svc, workflowCtrl: wfCtrl, logger: testLogger(t).WithFields()}

	// 1. Missing task_id
	msg := makeWSMessage(t, ws.ActionMCPGetTaskCompletionGate, map[string]interface{}{})
	resp, err := h.handleGetTaskCompletionGate(ctx, msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)

	// 2. Non-existent task
	msg = makeWSMessage(t, ws.ActionMCPGetTaskCompletionGate, map[string]interface{}{"task_id": "nonexistent"})
	resp, err = h.handleGetTaskCompletionGate(ctx, msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeNotFound)

	// 3. Valid task without criteria
	msg = makeWSMessage(t, ws.ActionMCPGetTaskCompletionGate, map[string]interface{}{"task_id": "task-cg1"})
	resp, err = h.handleGetTaskCompletionGate(ctx, msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)

	var snapshot models.TaskCompletionGateSnapshot
	require.NoError(t, json.Unmarshal(resp.Payload, &snapshot))
	assert.False(t, snapshot.Blocked)
	assert.Empty(t, snapshot.Criteria)
}

func seedApprovedPlanRevisionForTask(t *testing.T, ctx context.Context, db *sqlx.DB, taskID, revID string) {
	t.Helper()
	now := time.Now().UTC()
	writeVersion := "wv-" + revID
	_, err := db.ExecContext(ctx, `
		INSERT INTO task_plans (id, task_id, title, content, created_by, created_at, updated_at, write_version)
		VALUES (?, ?, 'Plan', 'Plan content', 'agent', ?, ?, ?)
		ON CONFLICT(task_id) DO UPDATE SET write_version = excluded.write_version
	`, "plan-"+taskID, taskID, now, now, writeVersion)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `
		INSERT INTO task_plan_revisions (id, task_id, revision_number, title, content, author_kind, author_name, workflow_step_id, created_at, updated_at, write_version)
		VALUES (?, ?, 1, 'Plan', 'Plan content', 'agent', 'architect', 'step-plan', ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET write_version = excluded.write_version
	`, revID, taskID, now, now, writeVersion)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `
		INSERT INTO task_plan_approval_receipts (id, task_id, plan_revision_id, write_version, decision, subject_edited, created_at)
		VALUES (?, ?, ?, ?, 'approve', 0, ?)
		ON CONFLICT(id) DO NOTHING
	`, "receipt-"+revID, taskID, revID, writeVersion, now)
	require.NoError(t, err)
}

func TestHandleSetTaskCompletionCriteria(t *testing.T) {
	svc, repo, wfCtrl, _, db := newTestTaskServiceWithWorkflowDB(t)
	ctx := context.Background()
	seedRunningTask(t, repo, "ws-cg2", "wf-cg2", "task-cg2", "sess-cg2", "step-1")
	seedApprovedPlanRevisionForTask(t, ctx, db, "task-cg2", "rev-1")

	h := &Handlers{taskSvc: svc, workflowCtrl: wfCtrl, logger: testLogger(t).WithFields()}

	// 1. Empty criteria rejected
	msg := makeWSMessage(t, ws.ActionMCPSetTaskCompletionCriteria, map[string]interface{}{
		"task_id":           "task-cg2",
		"expected_revision": 0,
		"criteria":          []models.TaskCompletionCriterion{},
	})
	resp, err := h.handleSetTaskCompletionCriteria(ctx, msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)

	// 2. Set valid criteria
	msg = makeWSMessage(t, ws.ActionMCPSetTaskCompletionCriteria, map[string]interface{}{
		"task_id":           "task-cg2",
		"expected_revision": 0,
		"plan_revision_id":  "rev-1",
		"sender_session_id": "sess-cg2",
		"criteria": []models.TaskCompletionCriterion{
			{ID: "inc-1", Description: "Implement increment 1"},
			{ID: "inc-2", Description: "Implement increment 2"},
		},
	})
	resp, err = h.handleSetTaskCompletionCriteria(ctx, msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)

	var snapshot models.TaskCompletionGateSnapshot
	require.NoError(t, json.Unmarshal(resp.Payload, &snapshot))
	assert.True(t, snapshot.Blocked)
	assert.Equal(t, int64(1), snapshot.Revision)
	assert.Equal(t, "rev-1", snapshot.PlanRevisionID)
	require.Len(t, snapshot.Criteria, 2)
	assert.Equal(t, "plan_increment", snapshot.Criteria[0].EvidenceSubject.Kind)
	assert.Equal(t, "inc-1", snapshot.Criteria[0].EvidenceSubject.ID)

	// 3. Stale expected_revision fails with CONFLICT
	msg = makeWSMessage(t, ws.ActionMCPSetTaskCompletionCriteria, map[string]interface{}{
		"task_id":           "task-cg2",
		"expected_revision": 0, // Current is 1
		"criteria": []models.TaskCompletionCriterion{
			{ID: "inc-1", Description: "Updated increment 1"},
		},
	})
	resp, err = h.handleSetTaskCompletionCriteria(ctx, msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeConflict)

	// 4. Agent attempting to weaken/remove unmet criteria without human confirmation fails with CONFLICT
	msg = makeWSMessage(t, ws.ActionMCPSetTaskCompletionCriteria, map[string]interface{}{
		"task_id":           "task-cg2",
		"expected_revision": 1,
		"sender_session_id": "sess-cg2",
		"criteria": []models.TaskCompletionCriterion{
			{ID: "inc-1", Description: "Implement increment 1"},
			// inc-2 removed!
		},
	})
	resp, err = h.handleSetTaskCompletionCriteria(ctx, msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeConflict)
}

func TestHandleVerifyTaskCompletionCriterion(t *testing.T) {
	svc, repo, wfCtrl, _, db := newTestTaskServiceWithWorkflowDB(t)
	ctx := context.Background()
	seedRunningTask(t, repo, "ws-cg3", "wf-cg3", "task-cg3", "sess-cg3", "step-1")
	seedApprovedPlanRevisionForTask(t, ctx, db, "task-cg3", "rev-cg3-1")

	h := &Handlers{taskSvc: svc, workflowCtrl: wfCtrl, logger: testLogger(t).WithFields()}

	// Register 1 criterion
	msg := makeWSMessage(t, ws.ActionMCPSetTaskCompletionCriteria, map[string]interface{}{
		"task_id":           "task-cg3",
		"expected_revision": 0,
		"plan_revision_id":  "rev-cg3-1",
		"criteria": []models.TaskCompletionCriterion{
			{ID: "inc-1", Description: "Implement increment 1"},
		},
	})
	resp, err := h.handleSetTaskCompletionCriteria(ctx, msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)

	// Missing summary fails
	msg = makeWSMessage(t, ws.ActionMCPVerifyTaskCompletionCriterion, map[string]interface{}{
		"task_id":           "task-cg3",
		"expected_revision": 1,
		"criterion_id":      "inc-1",
		"evidence":          models.TaskCompletionEvidence{Summary: ""},
	})
	resp, err = h.handleVerifyTaskCompletionCriterion(ctx, msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)

	// Verify criterion with sender_session_id attribution
	msg = makeWSMessage(t, ws.ActionMCPVerifyTaskCompletionCriterion, map[string]interface{}{
		"task_id":           "task-cg3",
		"expected_revision": 1,
		"criterion_id":      "inc-1",
		"sender_session_id": "sess-verifier-1",
		"evidence": models.TaskCompletionEvidence{
			Summary: "Unit and integration tests pass",
		},
	})
	resp, err = h.handleVerifyTaskCompletionCriterion(ctx, msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)

	var snapshot models.TaskCompletionGateSnapshot
	require.NoError(t, json.Unmarshal(resp.Payload, &snapshot))
	assert.False(t, snapshot.Blocked, "gate should unblock after all criteria are verified")
	assert.Equal(t, int64(1), snapshot.Revision)
	require.Len(t, snapshot.Criteria, 1)
	assert.Equal(t, int64(1), snapshot.Criteria[0].VerifiedRevision)
	require.NotNil(t, snapshot.Criteria[0].Evidence)
	assert.Equal(t, "sess-verifier-1", snapshot.Criteria[0].VerifierID)
	assert.Equal(t, "agent", snapshot.Criteria[0].VerifierKind)
}

func TestHandleManageTaskCompletionCriteria(t *testing.T) {
	svc, repo, wfCtrl, _, db := newTestTaskServiceWithWorkflowDB(t)
	ctx := context.Background()
	seedRunningTask(t, repo, "ws-cg4", "wf-cg4", "task-cg4", "sess-cg4", "step-1")
	seedApprovedPlanRevisionForTask(t, ctx, db, "task-cg4", "rev-cg4-1")

	h := &Handlers{taskSvc: svc, workflowCtrl: wfCtrl, logger: testLogger(t).WithFields()}

	// Invalid operation
	msg := makeWSMessage(t, ws.ActionMCPManageTaskCompletionCriteria, map[string]interface{}{
		"operation": "unknown",
		"task_id":   "task-cg4",
	})
	resp, err := h.handleManageTaskCompletionCriteria(ctx, msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)

	// Set via manage
	msg = makeWSMessage(t, ws.ActionMCPManageTaskCompletionCriteria, map[string]interface{}{
		"operation":         "set",
		"task_id":           "task-cg4",
		"expected_revision": 0,
		"plan_revision_id":  "rev-cg4-1",
		"criteria": []models.TaskCompletionCriterion{
			{ID: "inc-1", Description: "Implement increment 1"},
		},
	})
	resp, err = h.handleManageTaskCompletionCriteria(ctx, msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)

	// Get via manage
	msg = makeWSMessage(t, ws.ActionMCPManageTaskCompletionCriteria, map[string]interface{}{
		"operation": "get",
		"task_id":   "task-cg4",
	})
	resp, err = h.handleManageTaskCompletionCriteria(ctx, msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)

	// Verify via manage
	msg = makeWSMessage(t, ws.ActionMCPManageTaskCompletionCriteria, map[string]interface{}{
		"operation":         "verify",
		"task_id":           "task-cg4",
		"expected_revision": 1,
		"criterion_id":      "inc-1",
		"evidence": &models.TaskCompletionEvidence{
			Summary: "Verified through manage",
		},
	})
	resp, err = h.handleManageTaskCompletionCriteria(ctx, msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)
}

func TestHandleMoveTask_ImmediateTerminalBlockedByGate(t *testing.T) {
	svc, repo, wfCtrl, wfRepo, db := newTestTaskServiceWithWorkflowDB(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// Create workspace, workflow, steps (review position 2, done position 3)
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-term", Name: "ws-term", CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-term", WorkspaceID: "ws-term", Name: "wf-term", CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, wfRepo.CreateStep(ctx, &workflowmodels.WorkflowStep{
		ID: "step-review", WorkflowID: "wf-term", Name: "Review", Position: 2, CreatedAt: now, UpdatedAt: now,
		Events: workflowmodels.StepEvents{Transitions: []workflowmodels.StepTransition{{
			Name: "pass", Direction: workflowmodels.TransitionDirectionForward, ToStepID: "step-done",
		}}},
	}))
	require.NoError(t, wfRepo.CreateStep(ctx, &workflowmodels.WorkflowStep{
		ID: "step-done", WorkflowID: "wf-term", Name: "Done", Position: 3, CreatedAt: now, UpdatedAt: now,
		CompleteTaskOnEnter: true,
	}))

	// Create task in Review step, with an IDLE/completed session so it takes the immediate move path
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: "task-term", WorkspaceID: "ws-term", WorkflowID: "wf-term", WorkflowStepID: "step-review",
		Title: "Test terminal completion gate", State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "sess-term-idle", TaskID: "task-term", State: models.TaskSessionStateCompleted,
		IsPrimary: true, StartedAt: now, UpdatedAt: now,
	}))

	h := &Handlers{taskSvc: svc, workflowCtrl: wfCtrl, logger: testLogger(t).WithFields()}

	// Agent enrollment binds typed increments to an approved plan revision, so
	// the fixture must carry the approved plan receipt the contract requires.
	seedApprovedPlanRevisionForTask(t, ctx, db, "task-term", "rev-term-1")

	// 1. Set completion criteria (unverified)
	setMsg := makeWSMessage(t, ws.ActionMCPSetTaskCompletionCriteria, map[string]interface{}{
		"task_id":           "task-term",
		"expected_revision": 0,
		"plan_revision_id":  "rev-term-1",
		"criteria": []models.TaskCompletionCriterion{
			{ID: "inc-1", Description: "Implement increment 1"},
		},
	})
	setResp, err := h.handleSetTaskCompletionCriteria(ctx, setMsg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, setResp.Type)

	// 2. Attempt to move to Done via step_id -> should fail with TASK_COMPLETION_GATE_BLOCKED
	moveMsg := makeWSMessage(t, ws.ActionMCPMoveTask, map[string]interface{}{
		"task_id":          "task-term",
		"workflow_id":      "wf-term",
		"workflow_step_id": "step-done",
	})
	resp, err := h.handleMoveTask(ctx, moveMsg)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, ws.MessageTypeError, resp.Type)

	var ep ws.ErrorPayload
	require.NoError(t, json.Unmarshal(resp.Payload, &ep))
	assert.Equal(t, ws.ErrorCodeTaskCompletionGateBlocked, ep.Code)

	// Check structured details
	details := ep.Details
	require.NotNil(t, details, "error payload details must be a map of structured details")
	assert.Equal(t, ws.ErrorCodeTaskCompletionGateBlocked, details["code"])
	assert.NotEmpty(t, details["human_override_route"])
	assert.Equal(t, "step-review", details["current_step_id"])
	assert.Equal(t, "Review", details["current_step"])
	assert.NotEmpty(t, details["blockers"])
	assert.NotEmpty(t, details["unverified_criteria"])
	assert.NotEmpty(t, details["available_transitions"])

	// 3. Now verify the criterion
	verifyMsg := makeWSMessage(t, ws.ActionMCPVerifyTaskCompletionCriterion, map[string]interface{}{
		"task_id":           "task-term",
		"expected_revision": 1,
		"criterion_id":      "inc-1",
		"evidence":          models.TaskCompletionEvidence{Summary: "All increment 1 tests pass"},
	})
	verifyResp, err := h.handleVerifyTaskCompletionCriterion(ctx, verifyMsg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, verifyResp.Type)

	// 4. Move to Done should now succeed
	resp, err = h.handleMoveTask(ctx, moveMsg)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, ws.MessageTypeResponse, resp.Type)

	// Verify task state is completed
	task, err := svc.GetTask(ctx, "task-term")
	require.NoError(t, err)
	assert.Equal(t, "step-done", task.WorkflowStepID)
	assert.Equal(t, v1.TaskStateCompleted, task.State)
}

func TestHandleMoveTask_DeferredTerminalBlockedByGate(t *testing.T) {
	svc, repo, wfCtrl, wfRepo, db := newTestTaskServiceWithWorkflowDB(t)
	ctx := context.Background()
	now := time.Now().UTC()

	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-term2", Name: "ws-term2", CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-term2", WorkspaceID: "ws-term2", Name: "wf-term2", CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, wfRepo.CreateStep(ctx, &workflowmodels.WorkflowStep{
		ID: "step-review", WorkflowID: "wf-term2", Name: "Review", Position: 2, CreatedAt: now, UpdatedAt: now,
		Events: workflowmodels.StepEvents{Transitions: []workflowmodels.StepTransition{{
			Name: "pass", Direction: workflowmodels.TransitionDirectionForward, ToStepID: "step-done",
		}}},
	}))
	require.NoError(t, wfRepo.CreateStep(ctx, &workflowmodels.WorkflowStep{
		ID: "step-done", WorkflowID: "wf-term2", Name: "Done", Position: 3, CreatedAt: now, UpdatedAt: now,
		CompleteTaskOnEnter: true,
	}))

	// RUNNING session -> will hit deferMoveTask
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: "task-term2", WorkspaceID: "ws-term2", WorkflowID: "wf-term2", WorkflowStepID: "step-review",
		Title: "Test deferred terminal completion gate", State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "sess-term2-running", TaskID: "task-term2", State: models.TaskSessionStateRunning,
		IsPrimary: true, StartedAt: now, UpdatedAt: now,
	}))

	queue := &pendingMoveRecordingQueuer{}
	h := &Handlers{taskSvc: svc, workflowCtrl: wfCtrl, messageQueue: queue, logger: testLogger(t).WithFields()}

	seedApprovedPlanRevisionForTask(t, ctx, db, "task-term2", "rev-term-2")

	// Set completion criteria (unverified)
	setMsg := makeWSMessage(t, ws.ActionMCPSetTaskCompletionCriteria, map[string]interface{}{
		"task_id":           "task-term2",
		"expected_revision": 0,
		"plan_revision_id":  "rev-term-2",
		"criteria": []models.TaskCompletionCriterion{
			{ID: "inc-1", Description: "Implement increment 1"},
		},
	})
	setResp, err := h.handleSetTaskCompletionCriteria(ctx, setMsg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, setResp.Type)

	// Move via transition "pass" targeting Done
	moveMsg := makeWSMessage(t, ws.ActionMCPMoveTask, map[string]interface{}{
		"task_id":    "task-term2",
		"transition": "pass",
	})
	resp, err := h.handleMoveTask(ctx, moveMsg)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, ws.MessageTypeError, resp.Type)

	var ep ws.ErrorPayload
	require.NoError(t, json.Unmarshal(resp.Payload, &ep))
	assert.Equal(t, ws.ErrorCodeTaskCompletionGateBlocked, ep.Code)

	// Crucial: preflight must prevent queueing a pending move
	assert.Empty(t, queue.pendingMoves, "blocked completion gate must not enqueue a pending move")
}
