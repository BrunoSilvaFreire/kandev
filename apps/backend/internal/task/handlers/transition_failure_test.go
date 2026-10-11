package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/task/transitionfailure"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestHTTPGateBlockedMoveCarriesSharedFailureEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	snapshot := &models.TaskCompletionGateSnapshot{
		TaskID: "task-1", Revision: 3, PlanRevisionID: "rev-1", PlanWriteVersion: "w1",
		CurrentPlanWriteVersion: "w2", PlanStale: true, Blocked: true,
		Criteria: []models.TaskCompletionCriterion{{ID: "I1", Description: "Ships the diff", CriterionRevision: 1}},
		Blockers: []models.TaskCompletionBlocker{{CriterionID: "I1", Reason: "unverified"}},
	}
	repo := &completionGateResponseRepository{
		moveTaskConflictRepo: &moveTaskConflictRepo{task: &models.Task{ID: "task-1"}},
		snapshot:             snapshot,
	}
	log := newTestLogger(t)
	handlers := &TaskHandlers{
		service: service.NewService(service.Repos{Tasks: repo}, nil, log, service.RepositoryDiscoveryConfig{}),
		logger:  log,
	}
	response := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(response)
	ctx.Params = gin.Params{{Key: "id", Value: "task-1"}}
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/tasks/task-1/move", nil)

	handlers.attachTransitionFailure(ctx, "task-1", repository.ErrTaskCompletionGateBlocked)
	handlers.handleCompletionGateError(ctx, repository.ErrTaskCompletionGateBlocked)

	require.Equal(t, http.StatusConflict, response.Code)
	var body struct {
		Code    string                    `json:"code"`
		Failure transitionfailure.Failure `json:"failure"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	require.Equal(t, "task_completion_gate_blocked", body.Code)
	// The adapter serializes exactly what the classifier produced.
	want := handlers.service.ClassifyTransitionFailure(context.Background(), "task-1", repository.ErrTaskCompletionGateBlocked)
	require.Equal(t, want, body.Failure)
	require.Len(t, body.Failure.Reasons, 2)
	require.Equal(t, transitionfailure.RulePlanStale, body.Failure.Reasons[0].Rule)
	require.Equal(t, "I1", body.Failure.Reasons[1].CriterionID)
	require.Equal(t, "Ships the diff", body.Failure.Reasons[1].Description)
	require.EqualValues(t, 3, body.Failure.Gate.Revision)
}

func TestHTTPInternalMoveFailureHidesTheCauseAndNamesAnIncident(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := newTestLogger(t)
	repo := &moveTaskConflictRepo{task: &models.Task{ID: "task-1"}}
	handlers := &TaskHandlers{
		service: service.NewService(service.Repos{Tasks: repo}, nil, log, service.RepositoryDiscoveryConfig{}),
		logger:  log,
	}
	response := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(response)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/tasks/task-1/move", nil)
	cause := context.DeadlineExceeded

	handlers.attachTransitionFailure(ctx, "task-1", cause)
	handleSelectedMoveError(ctx, log, cause)

	require.Equal(t, http.StatusInternalServerError, response.Code)
	var body struct {
		Failure transitionfailure.Failure `json:"failure"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	require.Equal(t, transitionfailure.CodeInternal, body.Failure.Code)
	require.Equal(t, transitionfailure.ActionabilitySystem, body.Failure.Actionability)
	require.NotEmpty(t, body.Failure.IncidentID)
	require.NotContains(t, response.Body.String(), "deadline")
}

func TestWSMoveOfArchivedTaskExplainsTheRule(t *testing.T) {
	log := newTestLogger(t)
	archivedAt := time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)
	task := &models.Task{
		ID: "task-archived", WorkspaceID: "workspace-1", WorkflowID: "wf-source",
		WorkflowStepID: "step-source", ArchivedAt: &archivedAt,
	}
	repo := &moveTaskConflictRepo{task: task}
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	h := &TaskHandlers{service: svc, logger: log}
	request, err := ws.NewRequest("move-1", ws.ActionTaskMove, map[string]any{
		"id": task.ID, "workflow_id": "wf-target", "workflow_step_id": "step-target",
	})
	require.NoError(t, err)

	response, err := h.wsMoveTask(context.Background(), request)
	require.NoError(t, err)
	payload := wsWorkflowError(t, response)
	require.Equal(t, string(ws.ErrorCodeConflict), payload.Code)
	require.Equal(t, transitionfailure.CodeArchived, payload.Details["code"])
	require.Equal(t, transitionfailure.ActionabilityUser, payload.Details["actionability"])
	reasons, ok := payload.Details["reasons"].([]any)
	require.True(t, ok)
	require.Len(t, reasons, 1)
	require.Equal(t, transitionfailure.RuleArchived, reasons[0].(map[string]any)["rule"])
	require.NotEmpty(t, payload.Details["remediation"])
}
