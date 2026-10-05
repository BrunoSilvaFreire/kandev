package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/task/service"
)

type mockResumeHandoffOrchestrator struct {
	result       orchestrator.ResumeHandoffResult
	err          error
	sessionID    string
	instructions string
}

func (m *mockResumeHandoffOrchestrator) ResumeWithHandoff(_ context.Context, sessionID, instructions string) (orchestrator.ResumeHandoffResult, error) {
	m.sessionID = sessionID
	m.instructions = instructions
	return m.result, m.err
}

func setupResumeHandoffTestRouter(t *testing.T, orch any) *gin.Engine {
	gin.SetMode(gin.TestMode)
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	require.NoError(t, err)

	svc := service.NewService(service.Repos{}, nil, log, service.RepositoryDiscoveryConfig{})
	router := gin.New()
	handlers := &MessageHandlers{
		service:      svc,
		logger:       log,
		orchestrator: nil,
	}
	if o, ok := orch.(OrchestratorService); ok {
		handlers.orchestrator = o
	}
	handlers.registerHTTP(router)
	return router
}

// fullMockOrchestrator satisfies both OrchestratorService and resumeHandoffOrchestrator
type fullMockOrchestrator struct {
	resumeRetryOrchestrator
	mockResumeHandoffOrchestrator
}

var (
	_ OrchestratorService       = (*fullMockOrchestrator)(nil)
	_ resumeHandoffOrchestrator = (*fullMockOrchestrator)(nil)
)

func TestHttpResumeWithHandoff_Success(t *testing.T) {
	orch := &fullMockOrchestrator{
		mockResumeHandoffOrchestrator: mockResumeHandoffOrchestrator{
			result: orchestrator.ResumeHandoffResult{
				Handoff: "facts",
				Prompt:  "facts\n\n## Additional instructions\n\nkeep going",
				Sent:    true,
			},
		},
	}
	router := setupResumeHandoffTestRouter(t, orch)

	bodyBytes, _ := json.Marshal(map[string]string{"instructions": "keep going"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/task-sessions/session-123/resume-with-handoff", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "session-123", orch.sessionID)
	assert.Equal(t, "keep going", orch.instructions)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "facts", resp["handoff"])
	assert.Equal(t, "facts\n\n## Additional instructions\n\nkeep going", resp["prompt"])
	assert.Equal(t, true, resp["sent"])
}

func TestHttpResumeWithHandoff_EmptyBody(t *testing.T) {
	orch := &fullMockOrchestrator{
		mockResumeHandoffOrchestrator: mockResumeHandoffOrchestrator{
			result: orchestrator.ResumeHandoffResult{
				Handoff: "facts",
				Prompt:  "facts",
				Sent:    true,
			},
		},
	}
	router := setupResumeHandoffTestRouter(t, orch)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/task-sessions/session-123/resume-with-handoff", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "session-123", orch.sessionID)
	assert.Equal(t, "", orch.instructions)
}

func TestHttpResumeWithHandoff_PayloadTooLarge(t *testing.T) {
	orch := &fullMockOrchestrator{}
	router := setupResumeHandoffTestRouter(t, orch)

	largeInstructions := strings.Repeat("A", 33*1024) // 33 KiB > 32 KiB
	bodyBytes, _ := json.Marshal(map[string]string{"instructions": largeInstructions})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/task-sessions/session-123/resume-with-handoff", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHttpResumeWithHandoff_ErrorCodes(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantError  string
	}{
		{
			name:       "not eligible",
			err:        orchestrator.ErrSessionNotEligible,
			wantStatus: http.StatusBadRequest,
			wantError:  "not_eligible",
		},
		{
			name:       "provider unavailable",
			err:        orchestrator.ErrProviderUnavailable,
			wantStatus: http.StatusBadRequest,
			wantError:  "provider_unavailable",
		},
		{
			name:       "extraction failed",
			err:        orchestrator.ErrExtractionFailed,
			wantStatus: http.StatusInternalServerError,
			wantError:  "extraction_failed",
		},
		{
			name:       "extraction empty",
			err:        orchestrator.ErrResumeHandoffEmpty,
			wantStatus: http.StatusInternalServerError,
			wantError:  "extraction_failed",
		},
		{
			name:       "task not found",
			err:        repoerrors.ErrTaskNotFound,
			wantStatus: http.StatusNotFound,
			wantError:  "not found",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			orch := &fullMockOrchestrator{
				mockResumeHandoffOrchestrator: mockResumeHandoffOrchestrator{
					err: tc.err,
				},
			}
			router := setupResumeHandoffTestRouter(t, orch)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/task-sessions/session-123/resume-with-handoff", nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			require.Equal(t, tc.wantStatus, w.Code)
			var resp map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			assert.Equal(t, tc.wantError, resp["error"])
		})
	}
}

func TestHttpResumeWithHandoff_OrchestratorUnavailable(t *testing.T) {
	router := setupResumeHandoffTestRouter(t, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/task-sessions/session-123/resume-with-handoff", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "orchestrator unavailable", resp["error"])
}
