package handlers

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockStepPrimaryStepGetter struct {
	steps map[string]*wfmodels.WorkflowStep
}

func (m *mockStepPrimaryStepGetter) GetStep(_ context.Context, stepID string) (*wfmodels.WorkflowStep, error) {
	step, ok := m.steps[stepID]
	if !ok {
		return nil, nil
	}
	return step, nil
}

func (m *mockStepPrimaryStepGetter) GetNextStepByPosition(_ context.Context, _ string, _ int) (*wfmodels.WorkflowStep, error) {
	return nil, nil
}

func (m *mockStepPrimaryStepGetter) GetPreviousStepByPosition(_ context.Context, _ string, _ int) (*wfmodels.WorkflowStep, error) {
	return nil, nil
}

func (m *mockStepPrimaryStepGetter) GetWorkflowMeta(_ context.Context, _ string) (orchestrator.WorkflowMeta, error) {
	return orchestrator.WorkflowMeta{}, nil
}

func TestWsSetStepPrimarySession_BadRequest(t *testing.T) {
	handlers := setupOrchestratorHandlers(t)
	msg := &ws.Message{
		ID:      "m1",
		Action:  ws.ActionSessionSetStepPrimary,
		Payload: json.RawMessage(`invalid-json`),
	}
	response, err := handlers.wsSetStepPrimarySession(context.Background(), msg)
	require.NoError(t, err)
	payload := parseError(t, response)
	assert.Equal(t, ws.ErrorCodeBadRequest, payload.Code)
}

func TestWsSetStepPrimarySession_Validation(t *testing.T) {
	handlers := setupOrchestratorHandlers(t)

	tests := []struct {
		name        string
		payload     map[string]any
		wantMessage string
	}{
		{
			name:        "missing session_id",
			payload:     map[string]any{"step_id": "step-1"},
			wantMessage: "session_id is required",
		},
		{
			name:        "whitespace session_id",
			payload:     map[string]any{"session_id": "   ", "step_id": "step-1"},
			wantMessage: "session_id is required",
		},
		{
			name:        "missing step_id",
			payload:     map[string]any{"session_id": "sess-1"},
			wantMessage: "step_id is required",
		},
		{
			name:        "whitespace step_id",
			payload:     map[string]any{"session_id": "sess-1", "step_id": "  \t "},
			wantMessage: "step_id is required",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msg := createTestMessage(t, ws.ActionSessionSetStepPrimary, tc.payload)
			response, err := handlers.wsSetStepPrimarySession(context.Background(), msg)
			require.NoError(t, err)
			payload := parseError(t, response)
			assert.Equal(t, ws.ErrorCodeValidation, payload.Code)
			assert.Equal(t, tc.wantMessage, payload.Message)
		})
	}
}

func setupTestHandlersWithRepo(t *testing.T) (*Handlers, *taskrepo.Repository, *orchestrator.Service, context.Context) {
	t.Helper()
	ctx := context.Background()
	tmpDir := t.TempDir()
	dbConn, err := db.OpenSQLite(filepath.Join(tmpDir, "test.db"))
	require.NoError(t, err)
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })
	repo, cleanup, err := repository.Provide(sqlxDB, sqlxDB, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cleanup() })

	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console", OutputPath: "stderr"})
	require.NoError(t, err)
	svc := orchestrator.NewService(
		orchestrator.ServiceConfig{},
		bus.NewMemoryEventBus(log),
		&archivedLaunchAgentManager{},
		nil,
		repo,
		nil,
		nil,
		nil,
		log,
	)
	handlers := NewHandlers(svc, log)
	return handlers, repo, svc, ctx
}

func TestWsSetStepPrimarySession_ServiceError(t *testing.T) {
	handlers, _, _, ctx := setupTestHandlersWithRepo(t)
	// Calling with non-existent session triggers a service error
	msg := createTestMessage(t, ws.ActionSessionSetStepPrimary, map[string]any{
		"session_id": "sess-nonexistent",
		"step_id":    "step-1",
	})
	response, err := handlers.wsSetStepPrimarySession(ctx, msg)
	require.NoError(t, err)
	payload := parseError(t, response)
	assert.Equal(t, ws.ErrorCodeInternalError, payload.Code)
	assert.Contains(t, payload.Message, "Failed to set step primary session")
}

func TestWsSetStepPrimarySession_Success(t *testing.T) {
	handlers, repo, svc, ctx := setupTestHandlersWithRepo(t)

	now := time.Now().UTC()
	require.NoError(t, repo.CreateWorkspace(ctx, &taskmodels.Workspace{ID: "ws1", Name: "WS", CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateWorkflow(ctx, &taskmodels.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateTask(ctx, &taskmodels.Task{
		ID:             "task-1",
		WorkspaceID:    "ws1",
		WorkflowID:     "wf1",
		WorkflowStepID: "step-1",
		Title:          "Task 1",
		State:          v1.TaskStateInProgress,
		CreatedAt:      now,
		UpdatedAt:      now,
	}))
	require.NoError(t, repo.CreateTaskSession(ctx, &taskmodels.TaskSession{
		ID:        "sess-1",
		TaskID:    "task-1",
		State:     taskmodels.TaskSessionStateRunning,
		StartedAt: now,
		UpdatedAt: now,
	}))

	stepGetter := &mockStepPrimaryStepGetter{
		steps: map[string]*wfmodels.WorkflowStep{
			"step-1": {
				ID:         "step-1",
				WorkflowID: "wf1",
				Name:       "Step 1",
			},
		},
	}
	svc.SetWorkflowStepGetter(stepGetter)

	msg := createTestMessage(t, ws.ActionSessionSetStepPrimary, map[string]any{
		"session_id": "sess-1",
		"step_id":    "step-1",
	})
	response, err := handlers.wsSetStepPrimarySession(ctx, msg)
	require.NoError(t, err)
	assert.Equal(t, ws.ActionSessionSetStepPrimary, response.Action)

	var payload struct {
		Success bool `json:"success"`
	}
	require.NoError(t, json.Unmarshal(response.Payload, &payload))
	assert.True(t, payload.Success)

	// Verify task metadata
	dbTask, err := repo.GetTask(ctx, "task-1")
	require.NoError(t, err)
	assert.Equal(t, "sess-1", taskmodels.StepPrimarySessionID(dbTask.Metadata, "step-1"))
}
