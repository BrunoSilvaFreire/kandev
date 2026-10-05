package providerusage

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

// Handler serves the provider-usage HTTP surface.
type Handler struct {
	svc *Service
	log *logger.Logger
}

// RegisterRoutes mounts the provider-usage routes on the authenticated
// /api/v1 group shared with the agent-profile utilization endpoint.
func RegisterRoutes(router *gin.Engine, svc *Service, log *logger.Logger) *Handler {
	handler := &Handler{svc: svc, log: log}
	api := router.Group("/api/v1")
	api.GET("/provider-usage", handler.httpOverview)
	api.GET("/provider-usage/credentials", handler.httpCredentials)
	api.POST("/provider-usage/credentials/refresh", handler.httpRefreshCredentials)
	api.GET("/provider-usage/breakdown", handler.httpBreakdown)
	api.GET("/provider-usage/index", handler.httpIndexStatus)
	api.POST("/provider-usage/index", handler.httpStartIndex)
	api.PUT("/provider-usage/sources", handler.httpUpdateSources)
	return handler
}

func (h *Handler) httpOverview(c *gin.Context) {
	response, err := h.svc.Overview(c.Request.Context(), c.Query("range"))
	if err != nil {
		h.log.Error("provider usage overview failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "provider usage overview failed"})
		return
	}
	c.JSON(http.StatusOK, response)
}

// httpCredentials returns the storage descriptors for every provider
// credential Kandev supports, without ever echoing a secret value.
func (h *Handler) httpCredentials(c *gin.Context) {
	hints := h.svc.CredentialHints(c.Request.Context())
	if hints == nil {
		hints = []CredentialHint{}
	}
	c.JSON(http.StatusOK, hints)
}

// httpRefreshCredentials drops cached live quota so a credential saved through
// the UI is reflected on the next overview read.
func (h *Handler) httpRefreshCredentials(c *gin.Context) {
	h.svc.InvalidateQuotaCache()
	c.JSON(http.StatusOK, gin.H{"refreshed": true})
}

func (h *Handler) httpBreakdown(c *gin.Context) {
	query := parseBreakdownQuery(c)
	response, err := h.svc.Breakdown(c.Request.Context(), query)
	if err != nil {
		h.writeBreakdownError(c, err)
		return
	}
	c.JSON(http.StatusOK, response)
}

// writeBreakdownError maps a validation error to 400 and anything else to 500.
func (h *Handler) writeBreakdownError(c *gin.Context, err error) {
	var validation *breakdownValidationError
	if errors.As(err, &validation) {
		c.JSON(http.StatusBadRequest, gin.H{"error": validation.Error()})
		return
	}
	h.log.Error("provider usage breakdown failed", zap.Error(err))
	c.JSON(http.StatusInternalServerError, gin.H{"error": "provider usage breakdown failed"})
}

// breakdownValidationError marks a rejected closed-set query value so the
// handler can answer 400 rather than 500.
type breakdownValidationError struct{ message string }

func (e *breakdownValidationError) Error() string { return e.message }

func parseBreakdownQuery(c *gin.Context) BreakdownQuery {
	query := BreakdownQuery{
		Range:     c.Query("range"),
		GroupBy:   c.Query("group_by"),
		Provider:  c.Query("provider"),
		Model:     c.Query("model"),
		AgentType: c.Query("agent_type"),
		TaskID:    c.Query("task_id"),
		SessionID: c.Query("session_id"),
		Q:         c.Query("q"),
		Sort:      c.Query("sort"),
		Order:     c.Query("order"),
	}
	if limit, err := strconv.Atoi(c.Query("limit")); err == nil {
		query.Limit = limit
	}
	if offset, err := strconv.Atoi(c.Query("offset")); err == nil {
		query.Offset = offset
	}
	return query
}

func (h *Handler) httpIndexStatus(c *gin.Context) {
	status := h.svc.IndexStatus()
	sources, err := h.svc.Sources(c.Request.Context())
	if err != nil {
		sources = map[string]Source{}
	}
	// The plan's index-status contract carries the source list alongside the
	// job state; embed the status so the JSON stays flat.
	c.JSON(http.StatusOK, struct {
		IndexStatus
		Sources []SourceView `json:"sources"`
	}{IndexStatus: status, Sources: sourceViews(sources)})
}

func (h *Handler) httpStartIndex(c *gin.Context) {
	c.JSON(http.StatusOK, h.svc.StartIndex())
}

func (h *Handler) httpUpdateSources(c *gin.Context) {
	var request map[string]*bool
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	allowed := make(map[string]bool, len(ToggleableSources))
	for _, source := range ToggleableSources {
		allowed[source] = true
	}
	for source := range request {
		if !allowed[source] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unknown or non-toggleable source: " + source})
			return
		}
	}
	for source, enabled := range request {
		if enabled == nil {
			continue
		}
		if err := h.svc.SetSource(c.Request.Context(), source, *enabled); err != nil {
			h.log.Error("provider usage source update failed", zap.Error(err), zap.String("source", source))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "source update failed"})
			return
		}
	}
	sources, err := h.svc.Sources(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "source read failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"sources": sourceViews(sources)})
}
