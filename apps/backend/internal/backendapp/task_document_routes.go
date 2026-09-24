package backendapp

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/dashboard"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// mountTaskDocumentRoutes serves task documents on /api/v1/tasks/:id/documents...
// so a Kanban task's documents are readable with the Office feature off. The
// handler methods are shared with the Office route group unchanged; the guard
// below is what makes this widened surface safe.
func mountTaskDocumentRoutes(
	router *gin.Engine,
	docSvc *taskservice.DocumentService,
	taskSvc *taskservice.Service,
	homeDir string,
	log *logger.Logger,
) {
	if docSvc == nil || taskSvc == nil {
		log.Warn("task document routes disabled: document or task service is unavailable")
		return
	}
	group := router.Group("/api/v1")
	group.Use(authorizeTaskDocumentRoute(taskSvc))
	dashboard.RegisterDocumentRoutes(group, dashboard.NewDocumentHandler(docSvc, homeDir, log))
}

// authorizeTaskDocumentRoute resolves the route's :id and authorizes it before
// dispatch, failing closed on any resolver error. The Office document handlers
// contain no ownership check of their own, so mounting them under /api/v1
// without this guard would expose every task's documents to any authenticated
// user by guessed id. Read methods (GET, HEAD) check authz.ScopeWorkspaceRead;
// mutating methods (PUT, DELETE, POST) require authz.ScopeTaskWrite.
// The check is unconditional: an unscoped (auth-disabled) caller passes it
// inside AuthorizeTaskScope, not by skipping the guard.
func authorizeTaskDocumentRoute(taskSvc *taskservice.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if taskID := c.Param("id"); taskID != "" {
			scope := authz.ScopeWorkspaceRead
			if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
				scope = authz.ScopeTaskWrite
			}
			if err := taskSvc.AuthorizeTaskScope(c.Request.Context(), taskID, scope); err != nil {
				if taskservice.IsForbidden(err) {
					c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden"})
					return
				}
				c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "task not found"})
				return
			}
		}
		c.Next()
	}
}
