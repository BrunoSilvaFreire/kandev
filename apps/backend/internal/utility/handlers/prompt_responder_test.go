package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentctlutil "github.com/kandev/kandev/internal/agentctl/server/utility"
	"github.com/kandev/kandev/internal/utility/dto"
)

func newGinRecorder(accept string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	if accept != "" {
		c.Request.Header.Set("Accept", accept)
	}
	return c, rec
}

func TestPromptResponderStreamsProgressAndResult(t *testing.T) {
	c, rec := newGinRecorder("application/x-ndjson")
	w := newPromptResponder(c, acceptsNDJSON(c))
	w.beginStream()
	w.progressFrame(agentctlutil.PromptProgress{Phase: agentctlutil.PromptPhaseAnalyzing})
	w.finish(http.StatusOK, dto.ExecutePromptResponse{Success: true, Response: "hi"})

	body := rec.Body.String()
	assert.Contains(t, body, `"progress"`)
	assert.Contains(t, body, `"phase":"analyzing"`)
	assert.Contains(t, body, `"result"`)
	assert.Contains(t, body, `"success":true`)
	assert.Equal(t, "application/x-ndjson", rec.Header().Get("Content-Type"))
}

func TestPromptResponderStreamsFailureAsResultFrame(t *testing.T) {
	c, rec := newGinRecorder("application/x-ndjson")
	w := newPromptResponder(c, acceptsNDJSON(c))
	w.beginStream()
	w.finish(http.StatusInternalServerError, dto.ExecutePromptResponse{Error: "boom"})

	body := rec.Body.String()
	assert.Contains(t, body, `"result"`)
	assert.Contains(t, body, `"error":"boom"`)
}

func TestPromptResponderJSONModeUsesStatus(t *testing.T) {
	c, rec := newGinRecorder("application/json")
	w := newPromptResponder(c, acceptsNDJSON(c))
	require.False(t, acceptsNDJSON(c))
	w.finish(http.StatusServiceUnavailable, dto.ExecutePromptResponse{Error: "nope"})
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Contains(t, rec.Body.String(), `"error":"nope"`)
}
