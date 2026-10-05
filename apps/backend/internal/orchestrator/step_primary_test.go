package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type capturingStepPrimaryPublisher struct {
	updatedTasks []*models.Task
}

func (p *capturingStepPrimaryPublisher) PublishTaskUpdated(_ context.Context, task *models.Task, _ ...string) {
	p.updatedTasks = append(p.updatedTasks, task)
}

func (p *capturingStepPrimaryPublisher) PublishTaskStateChanged(_ context.Context, _ *models.Task, _ v1.TaskState) {
}

func (p *capturingStepPrimaryPublisher) PublishTaskActivityIfChanged(_ context.Context, _ string) {
}

func TestSetStepPrimarySession_Validation(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	stepGetter := newMockStepGetter()
	svc := createTestService(repo, stepGetter, newMockTaskRepo())

	// Empty session ID
	err := svc.SetStepPrimarySession(ctx, "", "step-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "session id is required")

	// Empty step ID
	err = svc.SetStepPrimarySession(ctx, "sess-1", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "step id is required")

	// Non-existent session
	err = svc.SetStepPrimarySession(ctx, "non-existent-session", "step-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get session")

	// Seed session and task
	now := time.Now().UTC()
	ws := &models.Workspace{ID: "ws1", Name: "WS", CreatedAt: now, UpdatedAt: now}
	require.NoError(t, repo.CreateWorkspace(ctx, ws))

	wf := &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}
	require.NoError(t, repo.CreateWorkflow(ctx, wf))

	task := &models.Task{
		ID:          "task-1",
		WorkspaceID: "ws1",
		WorkflowID:  "wf1",
		Title:       "Task 1",
		State:       v1.TaskStateInProgress,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	require.NoError(t, repo.CreateTask(ctx, task))

	sess := &models.TaskSession{
		ID:        "sess-1",
		TaskID:    "task-1",
		State:     models.TaskSessionStateRunning,
		StartedAt: now,
		UpdatedAt: now,
	}
	require.NoError(t, repo.CreateTaskSession(ctx, sess))

	// Step not found in stepGetter
	err = svc.SetStepPrimarySession(ctx, "sess-1", "missing-step")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "step not found")

	// Step belongs to another workflow
	stepGetter.steps["step-foreign"] = &wfmodels.WorkflowStep{
		ID:         "step-foreign",
		WorkflowID: "wf-other",
		Name:       "Foreign Step",
	}
	err = svc.SetStepPrimarySession(ctx, "sess-1", "step-foreign")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "step does not belong to task workflow")
}

func TestSetStepPrimarySession_SuccessAndOverwrite(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	stepGetter := newMockStepGetter()
	svc := createTestService(repo, stepGetter, newMockTaskRepo())

	publisher := &capturingStepPrimaryPublisher{}
	svc.SetTaskEventPublisher(publisher)

	now := time.Now().UTC()
	ws := &models.Workspace{ID: "ws1", Name: "WS", CreatedAt: now, UpdatedAt: now}
	require.NoError(t, repo.CreateWorkspace(ctx, ws))

	wf := &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}
	require.NoError(t, repo.CreateWorkflow(ctx, wf))

	stepGetter.steps["step-impl"] = &wfmodels.WorkflowStep{
		ID:         "step-impl",
		WorkflowID: "wf1",
		Name:       "Implement",
	}
	stepGetter.steps["step-review"] = &wfmodels.WorkflowStep{
		ID:         "step-review",
		WorkflowID: "wf1",
		Name:       "Review",
	}

	task := &models.Task{
		ID:             "task-1",
		WorkspaceID:    "ws1",
		WorkflowID:     "wf1",
		WorkflowStepID: "step-impl",
		Title:          "Task 1",
		State:          v1.TaskStateInProgress,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	require.NoError(t, repo.CreateTask(ctx, task))

	sess1 := &models.TaskSession{
		ID:        "sess-1",
		TaskID:    "task-1",
		State:     models.TaskSessionStateRunning,
		StartedAt: now,
		UpdatedAt: now,
	}
	require.NoError(t, repo.CreateTaskSession(ctx, sess1))

	sess2 := &models.TaskSession{
		ID:        "sess-2",
		TaskID:    "task-1",
		State:     models.TaskSessionStateRunning,
		StartedAt: now,
		UpdatedAt: now,
	}
	require.NoError(t, repo.CreateTaskSession(ctx, sess2))

	// 1. Set sess-1 as primary for step-impl
	require.NoError(t, svc.SetStepPrimarySession(ctx, "sess-1", "step-impl"))

	dbTask, err := repo.GetTask(ctx, "task-1")
	require.NoError(t, err)
	assert.Equal(t, "sess-1", models.StepPrimarySessionID(dbTask.Metadata, "step-impl"))
	assert.Empty(t, models.StepPrimarySessionID(dbTask.Metadata, "step-review"))
	require.Len(t, publisher.updatedTasks, 1)
	assert.Equal(t, "task-1", publisher.updatedTasks[0].ID)

	// 2. Overwrite step-impl with sess-2
	require.NoError(t, svc.SetStepPrimarySession(ctx, "sess-2", "step-impl"))

	dbTask, err = repo.GetTask(ctx, "task-1")
	require.NoError(t, err)
	assert.Equal(t, "sess-2", models.StepPrimarySessionID(dbTask.Metadata, "step-impl"))
	assert.Empty(t, models.StepPrimarySessionID(dbTask.Metadata, "step-review"))
	require.Len(t, publisher.updatedTasks, 2)

	// 3. Set sess-1 as primary for step-review (preserves step-impl)
	require.NoError(t, svc.SetStepPrimarySession(ctx, "sess-1", "step-review"))

	dbTask, err = repo.GetTask(ctx, "task-1")
	require.NoError(t, err)
	assert.Equal(t, "sess-2", models.StepPrimarySessionID(dbTask.Metadata, "step-impl"))
	assert.Equal(t, "sess-1", models.StepPrimarySessionID(dbTask.Metadata, "step-review"))
	require.Len(t, publisher.updatedTasks, 3)
}

func TestSetPrimarySession_RecordsStepPrimary(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	stepGetter := newMockStepGetter()
	svc := createTestService(repo, stepGetter, newMockTaskRepo())

	publisher := &capturingStepPrimaryPublisher{}
	svc.SetTaskEventPublisher(publisher)

	now := time.Now().UTC()
	ws := &models.Workspace{ID: "ws1", Name: "WS", CreatedAt: now, UpdatedAt: now}
	require.NoError(t, repo.CreateWorkspace(ctx, ws))

	wf := &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}
	require.NoError(t, repo.CreateWorkflow(ctx, wf))

	task := &models.Task{
		ID:             "task-1",
		WorkspaceID:    "ws1",
		WorkflowID:     "wf1",
		WorkflowStepID: "step-impl",
		Title:          "Task 1",
		State:          v1.TaskStateInProgress,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	require.NoError(t, repo.CreateTask(ctx, task))

	sess1 := &models.TaskSession{
		ID:        "sess-1",
		TaskID:    "task-1",
		State:     models.TaskSessionStateRunning,
		StartedAt: now,
		UpdatedAt: now,
	}
	require.NoError(t, repo.CreateTaskSession(ctx, sess1))

	// Call SetPrimarySession - should set primary in repo AND record step-impl designation
	require.NoError(t, svc.SetPrimarySession(ctx, "sess-1"))

	dbSess, err := repo.GetTaskSession(ctx, "sess-1")
	require.NoError(t, err)
	assert.True(t, dbSess.IsPrimary)

	dbTask, err := repo.GetTask(ctx, "task-1")
	require.NoError(t, err)
	assert.Equal(t, "sess-1", models.StepPrimarySessionID(dbTask.Metadata, "step-impl"))

	// Case with empty WorkflowStepID: only sets primary, no step designation
	taskNoStep := &models.Task{
		ID:          "task-2",
		WorkspaceID: "ws1",
		WorkflowID:  "wf1",
		Title:       "Task 2",
		State:       v1.TaskStateInProgress,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	require.NoError(t, repo.CreateTask(ctx, taskNoStep))

	sess2 := &models.TaskSession{
		ID:        "sess-2",
		TaskID:    "task-2",
		State:     models.TaskSessionStateRunning,
		StartedAt: now,
		UpdatedAt: now,
	}
	require.NoError(t, repo.CreateTaskSession(ctx, sess2))

	require.NoError(t, svc.SetPrimarySession(ctx, "sess-2"))

	dbSess2, err := repo.GetTaskSession(ctx, "sess-2")
	require.NoError(t, err)
	assert.True(t, dbSess2.IsPrimary)

	dbTask2, err := repo.GetTask(ctx, "task-2")
	require.NoError(t, err)
	assert.Nil(t, models.StepPrimarySessions(dbTask2.Metadata))
}
