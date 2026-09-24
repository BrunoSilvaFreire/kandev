package backendapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/common/logger"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

func newTaskDocumentTestLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	return log
}

// newTaskDocumentTestRouter mounts the task-scoped document routes behind a
// fixed request identity. An empty userID leaves the request unscoped, which is
// the auth-disabled path.
func newTaskDocumentTestRouter(
	t *testing.T,
	taskSvc *taskservice.Service,
	docSvc *taskservice.DocumentService,
	log *logger.Logger,
	userID string,
) *gin.Engine {
	t.Helper()
	router := gin.New()
	if userID != "" {
		router.Use(func(c *gin.Context) {
			authn.SetOnGin(c, authn.Identity{UserID: userID, Role: authn.RoleMember})
			c.Next()
		})
	}
	mountTaskDocumentRoutes(router, docSvc, taskSvc, t.TempDir(), log)
	return router
}

func TestTaskDocumentRoutes_MountedUnderAPIV1(t *testing.T) {
	gin.SetMode(gin.TestMode)
	taskSvc, taskRepo, _ := newRunSubscriptionCheckHarness(t)
	log := newTaskDocumentTestLogger(t)
	docSvc := taskservice.NewDocumentService(taskRepo, log)

	router := newTaskDocumentTestRouter(t, taskSvc, docSvc, log, "")

	want := []string{
		"GET /api/v1/tasks/:id/documents",
		"GET /api/v1/tasks/:id/documents/:key",
		"PUT /api/v1/tasks/:id/documents/:key",
		"DELETE /api/v1/tasks/:id/documents/:key",
		"GET /api/v1/tasks/:id/documents/:key/revisions",
		"POST /api/v1/tasks/:id/documents/:key/revisions/:revId/restore",
		"POST /api/v1/tasks/:id/documents/:key/upload",
		"GET /api/v1/tasks/:id/documents/:key/download",
	}
	registered := map[string]bool{}
	for _, route := range router.Routes() {
		registered[route.Method+" "+route.Path] = true
	}
	for _, route := range want {
		if !registered[route] {
			t.Errorf("task document route %q is not mounted", route)
		}
	}
}

func TestTaskDocumentRoutes_GuardDeniesForeignTask(t *testing.T) {
	gin.SetMode(gin.TestMode)
	taskSvc, taskRepo, _ := newRunSubscriptionCheckHarness(t)
	log := newTaskDocumentTestLogger(t)
	docSvc := taskservice.NewDocumentService(taskRepo, log)

	ctx := context.Background()
	if err := taskRepo.CreateWorkspace(ctx, &taskmodels.Workspace{
		ID: "ws-a", Name: "Workspace A", OwnerID: "owner-a",
	}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	now := time.Now().UTC()
	if err := taskRepo.CreateTask(ctx, &taskmodels.Task{
		ID: "task-a", WorkspaceID: "ws-a", Title: "Owned task",
		State: "CREATED", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}

	getDocuments := func(userID string) *httptest.ResponseRecorder {
		router := newTaskDocumentTestRouter(t, taskSvc, docSvc, log, userID)
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/task-a/documents", nil)
		router.ServeHTTP(rec, req)
		return rec
	}

	if got := getDocuments("owner-a"); got.Code != http.StatusOK {
		t.Fatalf("owner request status = %d, want 200 (body: %s)", got.Code, got.Body.String())
	}
	if got := getDocuments("owner-b"); got.Code != http.StatusNotFound {
		t.Fatalf("foreign request status = %d, want 404 (body: %s)", got.Code, got.Body.String())
	}
	// Auth disabled: no identity means unscoped, so the guard must not be the
	// thing that blocks an ordinary request.
	if got := getDocuments(""); got.Code != http.StatusOK {
		t.Fatalf("unscoped request status = %d, want 200 (body: %s)", got.Code, got.Body.String())
	}
}

func TestTaskDocumentRoutes_GuardDeniesUnknownTask(t *testing.T) {
	gin.SetMode(gin.TestMode)
	taskSvc, taskRepo, _ := newRunSubscriptionCheckHarness(t)
	log := newTaskDocumentTestLogger(t)
	docSvc := taskservice.NewDocumentService(taskRepo, log)

	router := newTaskDocumentTestRouter(t, taskSvc, docSvc, log, "owner-a")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/does-not-exist/documents", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown task status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestTaskDocumentRoutes_GuardDeniesReadOnlyUserOnMutatingMethods(t *testing.T) {
	gin.SetMode(gin.TestMode)
	taskSvc, taskRepo, _ := newRunSubscriptionCheckHarness(t)
	log := newTaskDocumentTestLogger(t)
	docSvc := taskservice.NewDocumentService(taskRepo, log)

	ctx := context.Background()
	if err := taskRepo.CreateWorkspace(ctx, &taskmodels.Workspace{
		ID: "ws-write", Name: "Workspace Write", OwnerID: "owner-w",
	}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := taskRepo.UpsertWorkspaceMember(ctx, &taskmodels.WorkspaceMember{
		WorkspaceID: "ws-write", UserID: "viewer-w", Role: string(authz.WorkspaceRoleViewer),
	}); err != nil {
		t.Fatalf("upsert member: %v", err)
	}
	now := time.Now().UTC()
	if err := taskRepo.CreateTask(ctx, &taskmodels.Task{
		ID: "task-w", WorkspaceID: "ws-write", Title: "Guarded task",
		State: "CREATED", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}

	putDocument := func(userID string) *httptest.ResponseRecorder {
		router := newTaskDocumentTestRouter(t, taskSvc, docSvc, log, userID)
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/v1/tasks/task-w/documents/notes", strings.NewReader(`{"content":"test"}`))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(rec, req)
		return rec
	}

	// Viewer can read the task, but lacks ScopeTaskWrite on mutating method.
	if got := putDocument("viewer-w"); got.Code != http.StatusForbidden {
		t.Fatalf("viewer PUT status = %d, want 403 (body: %s)", got.Code, got.Body.String())
	}
	// Owner has ScopeTaskWrite, so the guard passes.
	if got := putDocument("owner-w"); got.Code != http.StatusOK {
		t.Fatalf("owner PUT status = %d, want 200 (body: %s)", got.Code, got.Body.String())
	}
}
