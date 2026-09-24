package utility

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Handler provides HTTP handlers for inference operations.
type Handler struct {
	executor *ACPInferenceExecutor
	logger   *zap.Logger
}

// NewHandler creates a new inference handler.
func NewHandler(_ string, logger *zap.Logger) *Handler {
	return &Handler{
		executor: NewACPInferenceExecutor(logger),
		logger:   logger,
	}
}

// RegisterRoutes registers the inference routes on the given router group.
func (h *Handler) RegisterRoutes(api *gin.RouterGroup) {
	aux := api.Group("/inference")
	aux.POST("/prompt", h.handlePrompt)
	aux.POST("/probe", h.handleProbe)
}

// handleProbe handles POST /api/v1/inference/probe
func (h *Handler) handleProbe(c *gin.Context) {
	var req ProbeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ProbeResponse{
			Success: false,
			Error:   "invalid request: " + err.Error(),
		})
		return
	}

	h.logger.Info("executing ACP probe", zap.String("agent_id", req.AgentID))

	resp, err := h.executor.Probe(c.Request.Context(), &req)
	if err != nil {
		h.logger.Error("ACP probe failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ProbeResponse{
			Success: false,
			Error:   "probe execution failed",
		})
		return
	}

	if !resp.Success {
		h.logger.Warn("ACP probe unsuccessful", zap.String("error", resp.Error))
	} else {
		h.logger.Info("ACP probe completed",
			zap.Int("duration_ms", resp.DurationMs),
			zap.Int("models", len(resp.Models)),
			zap.Int("modes", len(resp.Modes)))
	}

	c.JSON(http.StatusOK, resp)
}

// handlePrompt handles POST /api/v1/inference/prompt
func (h *Handler) handlePrompt(c *gin.Context) {
	var req PromptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, PromptResponse{
			Success: false,
			Error:   "invalid request: " + err.Error(),
		})
		return
	}

	h.logger.Info("executing inference prompt",
		zap.String("agent_id", req.AgentID),
		zap.String("model", req.Model),
	)

	if req.StreamProgress {
		h.streamPrompt(c, &req)
		return
	}

	resp, err := h.executor.Execute(c.Request.Context(), &req)
	if err != nil {
		h.logger.Error("inference prompt failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, PromptResponse{
			Success: false,
			Error:   "inference execution failed",
		})
		return
	}

	if !resp.Success {
		h.logger.Warn("inference prompt unsuccessful", zap.String("error", resp.Error))
	} else {
		h.logger.Info("inference prompt completed", zap.Int("duration_ms", resp.DurationMs))
	}

	c.JSON(http.StatusOK, resp)
}

// streamPrompt answers an inference prompt with newline-delimited JSON:
// {"progress":{...}} frames while the agent works, then a final
// {"result":<PromptResponse>} frame on success and on every failure.
func (h *Handler) streamPrompt(c *gin.Context, req *PromptRequest) {
	c.Header("Content-Type", "application/x-ndjson")
	c.Header("Cache-Control", "no-cache")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	flusher, _ := c.Writer.(http.Flusher)
	var mu sync.Mutex
	closed := false
	writeFrame := func(v any) {
		data, err := json.Marshal(v)
		if err != nil {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if closed {
			return
		}
		_, _ = c.Writer.Write(append(data, '\n'))
		if flusher != nil {
			flusher.Flush()
		}
	}
	// writeResult writes the terminal frame and closes the stream so a late ACP
	// notification (handled on the SDK's own goroutine) cannot write to the
	// returned gin.Context.
	writeResult := func(resp *PromptResponse) {
		writeFrame(map[string]any{"result": resp})
		mu.Lock()
		closed = true
		mu.Unlock()
	}

	reporter := func(p PromptProgress) { writeFrame(map[string]any{"progress": p}) }
	ctx := WithProgressReporter(c.Request.Context(), reporter)

	resp, err := h.executor.Execute(ctx, req)
	if err != nil {
		h.logger.Error("inference prompt failed", zap.Error(err))
		writeResult(&PromptResponse{Success: false, Error: "inference execution failed"})
		return
	}
	if resp == nil {
		writeResult(&PromptResponse{Success: false, Error: "inference execution returned no response"})
		return
	}
	if !resp.Success {
		h.logger.Warn("inference prompt unsuccessful", zap.String("error", resp.Error))
	} else {
		h.logger.Info("inference prompt completed", zap.Int("duration_ms", resp.DurationMs))
	}
	writeResult(resp)
}
