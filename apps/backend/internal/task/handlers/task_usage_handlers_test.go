package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
)

// usageTotalsRepo backs the task-cost-ledger HTTP handler tests
// (docs/specs/task-cost-ledger/spec.md AC-18). It reuses foreignSessionRepo's
// task-b/sess-b-owned-by-user-b fixture and stubs the two aggregate reads.
type usageTotalsRepo struct {
	foreignSessionRepo
	taskTotals    *models.TaskUsageTotals
	sessionTotals *models.TaskUsageTotals
	groups        []*models.TaskUsageTotalsGroup
}

func (r *usageTotalsRepo) GetTaskUsageTotals(context.Context, string) (*models.TaskUsageTotals, error) {
	return r.taskTotals, nil
}

func (r *usageTotalsRepo) GetSessionUsageTotals(context.Context, string) (*models.TaskUsageTotals, error) {
	return r.sessionTotals, nil
}

func (r *usageTotalsRepo) ListTaskUsageTotalGroups(context.Context, string) ([]*models.TaskUsageTotalsGroup, error) {
	return r.groups, nil
}

func newUsageTotalsHandlers(t *testing.T, taskTotals, sessionTotals *models.TaskUsageTotals) *TaskHandlers {
	t.Helper()
	return newUsageBreakdownHandlers(t, taskTotals, sessionTotals, nil)
}

func newUsageBreakdownHandlers(
	t *testing.T,
	taskTotals, sessionTotals *models.TaskUsageTotals,
	groups []*models.TaskUsageTotalsGroup,
) *TaskHandlers {
	t.Helper()
	log := newTestLogger(t)
	repo := &usageTotalsRepo{taskTotals: taskTotals, sessionTotals: sessionTotals, groups: groups}
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo, Usage: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	return &TaskHandlers{service: svc, logger: log}
}

func taskUsageRequestAs(t *testing.T, path, userID string, params gin.Params) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, path, nil)
	c.Request = c.Request.WithContext(
		authn.WithIdentity(c.Request.Context(), authn.Identity{UserID: userID, Role: authn.RoleMember}),
	)
	c.Params = params
	return c, rec
}

// TestHTTPGetTaskUsageTotalsDeniesForeignTaskWith404 pins the scoping
// requirement CLAUDE.md's backend security-boundary rule adds to any new
// task-keyed route: a foreign owner is denied, the real owner passes.
func TestHTTPGetTaskUsageTotalsDeniesForeignTaskWith404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := newUsageTotalsHandlers(t, &models.TaskUsageTotals{OutputTokensComplete: true}, nil)

	c, rec := taskUsageRequestAs(t, "/api/v1/tasks/task-b/usage", "user-a", gin.Params{{Key: "id", Value: "task-b"}})
	h.httpGetTaskUsageTotals(c)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
	}

	ownerCtx, ownerRec := taskUsageRequestAs(t, "/api/v1/tasks/task-b/usage", "user-b", gin.Params{{Key: "id", Value: "task-b"}})
	h.httpGetTaskUsageTotals(ownerCtx)
	if ownerRec.Code != http.StatusOK {
		t.Fatalf("owner status = %d, want 200 (body: %s)", ownerRec.Code, ownerRec.Body.String())
	}
}

// TestHTTPGetTaskUsageTotalsZeroUsageReturns200WithZeroedBody pins AC-20: a
// known task with no recorded usage answers 200 with zeroed totals, not an
// error, and the response identifies its own scope.
func TestHTTPGetTaskUsageTotalsZeroUsageReturns200WithZeroedBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := newUsageTotalsHandlers(t, &models.TaskUsageTotals{OutputTokensComplete: true}, nil)

	c, rec := taskUsageRequestAs(t, "/api/v1/tasks/task-b/usage", "user-b", gin.Params{{Key: "id", Value: "task-b"}})
	h.httpGetTaskUsageTotals(c)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}

	var body dto.TaskUsageTotalsDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Scope != dto.TaskUsageTotalsScopeTask || body.ScopeID != "task-b" {
		t.Fatalf("scope = (%q, %q), want (task, task-b)", body.Scope, body.ScopeID)
	}
	if body.EventCount != 0 {
		t.Fatalf("EventCount = %d, want 0", body.EventCount)
	}
	if !body.OutputTokensComplete {
		t.Error("OutputTokensComplete = false, want true for zero usage")
	}
}

// TestHTTPGetTaskSessionUsageTotalsDeniesForeignSessionWith404 mirrors the
// task-scoped denial test for the session-scoped route.
func TestHTTPGetTaskSessionUsageTotalsDeniesForeignSessionWith404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := newUsageTotalsHandlers(t, nil, &models.TaskUsageTotals{OutputTokensComplete: true})
	params := gin.Params{{Key: "id", Value: "task-b"}, {Key: "sessionId", Value: "sess-b"}}

	c, rec := taskUsageRequestAs(t, "/api/v1/tasks/task-b/sessions/sess-b/usage", "user-a", params)
	h.httpGetTaskSessionUsageTotals(c)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
	}

	ownerCtx, ownerRec := taskUsageRequestAs(t, "/api/v1/tasks/task-b/sessions/sess-b/usage", "user-b", params)
	h.httpGetTaskSessionUsageTotals(ownerCtx)
	if ownerRec.Code != http.StatusOK {
		t.Fatalf("owner status = %d, want 200 (body: %s)", ownerRec.Code, ownerRec.Body.String())
	}
}

// TestHTTPGetTaskSessionUsageTotalsReturnsSessionScopedBody proves the
// session route stamps scope=session and the path's sessionId, not the
// task's, into the response.
func TestHTTPGetTaskSessionUsageTotalsReturnsSessionScopedBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := newUsageTotalsHandlers(t, nil, &models.TaskUsageTotals{EventCount: 3, OutputTokensComplete: true})
	params := gin.Params{{Key: "id", Value: "task-b"}, {Key: "sessionId", Value: "sess-b"}}

	c, rec := taskUsageRequestAs(t, "/api/v1/tasks/task-b/sessions/sess-b/usage", "user-b", params)
	h.httpGetTaskSessionUsageTotals(c)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}

	var body dto.TaskUsageTotalsDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Scope != dto.TaskUsageTotalsScopeSession || body.ScopeID != "sess-b" {
		t.Fatalf("scope = (%q, %q), want (session, sess-b)", body.Scope, body.ScopeID)
	}
	if body.EventCount != 3 {
		t.Fatalf("EventCount = %d, want 3", body.EventCount)
	}
}

// TestHTTPGetTaskUsageBreakdownReturnsGroups pins the breakdown route's shape:
// the task total plus the finest-grain groups, with a deleted session's
// session_id serialized as JSON null and scope stamped "group".
func TestHTTPGetTaskUsageBreakdownReturnsGroups(t *testing.T) {
	gin.SetMode(gin.TestMode)
	group := &models.TaskUsageTotalsGroup{
		SessionID:      nil, // deleted session
		AgentProfileID: "profile-1",
		AgentType:      "claude",
		Model:          "model-x",
		Provider:       "provider-1",
		Totals:         models.TaskUsageTotals{EventCount: 2, TokensIn: 200, OutputTokensComplete: true},
	}
	h := newUsageBreakdownHandlers(t,
		&models.TaskUsageTotals{EventCount: 2, TokensIn: 200, OutputTokensComplete: true},
		nil,
		[]*models.TaskUsageTotalsGroup{group},
	)

	c, rec := taskUsageRequestAs(t, "/api/v1/tasks/task-b/usage/breakdown", "user-b", gin.Params{{Key: "id", Value: "task-b"}})
	h.httpGetTaskUsageBreakdown(c)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}

	var body dto.TaskUsageBreakdownDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.TaskID != "task-b" {
		t.Errorf("TaskID = %q, want task-b", body.TaskID)
	}
	if body.Task.Scope != dto.TaskUsageTotalsScopeTask || body.Task.EventCount != 2 {
		t.Errorf("task = %+v, want scope task with 2 events", body.Task)
	}
	if len(body.Groups) != 1 {
		t.Fatalf("len(groups) = %d, want 1", len(body.Groups))
	}
	if body.Groups[0].SessionID != nil {
		t.Errorf("SessionID = %v, want nil (deleted session)", body.Groups[0].SessionID)
	}
	if body.Groups[0].Totals.Scope != dto.TaskUsageTotalsScopeGroup {
		t.Errorf("group scope = %q, want group", body.Groups[0].Totals.Scope)
	}

	// The raw JSON must keep session_id as an explicit null, not omit it.
	if !strings.Contains(rec.Body.String(), `"session_id":null`) {
		t.Errorf("body does not contain an explicit session_id null: %s", rec.Body.String())
	}
}

// TestHTTPGetTaskUsageBreakdownEmptyGroupsSerializesAsArray pins that a task
// with no usage returns `"groups":[]`, never `"groups":null`.
func TestHTTPGetTaskUsageBreakdownEmptyGroupsSerializesAsArray(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := newUsageBreakdownHandlers(t, &models.TaskUsageTotals{OutputTokensComplete: true}, nil, nil)

	c, rec := taskUsageRequestAs(t, "/api/v1/tasks/task-b/usage/breakdown", "user-b", gin.Params{{Key: "id", Value: "task-b"}})
	h.httpGetTaskUsageBreakdown(c)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"groups":[]`) {
		t.Errorf("body does not contain an empty groups array: %s", rec.Body.String())
	}
}

// TestHTTPGetTaskUsageBreakdownDeniesForeignTaskWith404 mirrors the task-scope
// denial test for the breakdown route.
func TestHTTPGetTaskUsageBreakdownDeniesForeignTaskWith404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := newUsageBreakdownHandlers(t, &models.TaskUsageTotals{OutputTokensComplete: true}, nil, nil)

	c, rec := taskUsageRequestAs(t, "/api/v1/tasks/task-b/usage/breakdown", "user-a", gin.Params{{Key: "id", Value: "task-b"}})
	h.httpGetTaskUsageBreakdown(c)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
	}

	ownerCtx, ownerRec := taskUsageRequestAs(t, "/api/v1/tasks/task-b/usage/breakdown", "user-b", gin.Params{{Key: "id", Value: "task-b"}})
	h.httpGetTaskUsageBreakdown(ownerCtx)
	if ownerRec.Code != http.StatusOK {
		t.Fatalf("owner status = %d, want 200 (body: %s)", ownerRec.Code, ownerRec.Body.String())
	}
}
