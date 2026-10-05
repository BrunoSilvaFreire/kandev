package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/searchcursor"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func taskSearchHit(id, session string, withStep bool) *models.TaskMessageSearchHit {
	hit := &models.TaskMessageSearchHit{
		Message: &models.Message{
			ID: id, TaskSessionID: session, Content: "needle body",
			AuthorType: models.MessageAuthorUser, Type: models.MessageTypeMessage,
			CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		OrderKey: "2026-01-01 00:00:00.000000",
	}
	if withStep {
		hit.WorkflowStepID = "step-x"
	}
	return hit
}

func TestWSSearchTaskMessagesActiveFirstAndCursorRoundTrip(t *testing.T) {
	repo := &messageListRepo{
		taskSearchHits: []*models.TaskMessageSearchHit{
			{
				Message:  taskSearchHit("m-1", "sess-a", false).Message,
				Session:  &models.TaskSession{ID: "sess-a", Name: "Alpha", AgentProfileID: "p1"},
				OrderKey: "2026-01-01 00:00:01.000000",
			},
			{
				Message:  taskSearchHit("m-2", "sess-b", false).Message,
				OrderKey: "2026-01-01 00:00:00.000000",
			},
		},
		taskSearchHasMore: true,
	}
	h := newMessageListHandlers(t, repo)
	ctx := context.Background()

	resp, err := h.wsSearchMessages(ctx, wsWorkflowRequest(t, ws.ActionMessageSearch, map[string]any{
		"task_id": "task-x", "active_session_id": "sess-a", "query": "needle", "limit": 2,
	}))
	require.NoError(t, err)
	var out dto.SearchMessagesResponse
	wsWorkflowResponse(t, resp, &out)
	require.Len(t, out.Hits, 2)
	require.True(t, out.HasMore)
	require.NotEmpty(t, out.NextCursor)
	require.Equal(t, "Alpha", out.Hits[0].SessionName)
	require.Equal(t, "p1", out.Hits[0].AgentProfileID)
	// The second hit has no session row and no turn stamp: it stays unlabeled.
	require.Equal(t, "sess-b", out.Hits[1].SessionID)
	require.Nil(t, out.Hits[1].WorkflowStepID)
	require.Len(t, repo.taskSearchOptions, 1)
	require.Nil(t, repo.taskSearchOptions[0].Cursor)

	binding := searchcursor.Binding{TaskID: "task-x", ActiveSessionID: "sess-a", Query: "needle"}
	decoded, err := searchcursor.Decode(out.NextCursor, binding)
	require.NoError(t, err)
	require.Equal(t, "m-2", decoded.ID)
	require.Equal(t, 1, decoded.Bucket, "sess-b is not the active session")

	resp2, err := h.wsSearchMessages(ctx, wsWorkflowRequest(t, ws.ActionMessageSearch, map[string]any{
		"task_id": "task-x", "active_session_id": "sess-a", "query": "needle", "cursor": out.NextCursor,
	}))
	require.NoError(t, err)
	var out2 dto.SearchMessagesResponse
	wsWorkflowResponse(t, resp2, &out2)
	require.Len(t, repo.taskSearchOptions, 2)
	require.NotNil(t, repo.taskSearchOptions[1].Cursor)
	require.Equal(t, "m-2", repo.taskSearchOptions[1].Cursor.ID)
	require.Equal(t, "2026-01-01 00:00:00.000000", repo.taskSearchOptions[1].Cursor.Key)
}

func TestWSSearchTaskMessagesRequiresExactlyOneScope(t *testing.T) {
	repo := &messageListRepo{}
	h := newMessageListHandlers(t, repo)
	ctx := context.Background()

	both, err := h.wsSearchMessages(ctx, wsWorkflowRequest(t, ws.ActionMessageSearch, map[string]any{
		"session_id": "sess-b", "task_id": "task-x", "query": "needle",
	}))
	require.NoError(t, err)
	require.Equal(t, "exactly one of session_id or task_id is required", wsWorkflowError(t, both).Message)

	neither, err := h.wsSearchMessages(ctx, wsWorkflowRequest(t, ws.ActionMessageSearch, map[string]any{
		"query": "needle",
	}))
	require.NoError(t, err)
	require.Equal(t, "session_id is required", wsWorkflowError(t, neither).Message)
	require.Zero(t, repo.taskSearchCalls)
}

func TestWSSearchTaskMessagesRejectsInvalidCursor(t *testing.T) {
	repo := &messageListRepo{}
	h := newMessageListHandlers(t, repo)
	ctx := context.Background()

	garbage, err := h.wsSearchMessages(ctx, wsWorkflowRequest(t, ws.ActionMessageSearch, map[string]any{
		"task_id": "task-x", "active_session_id": "sess-a", "query": "needle", "cursor": "!!!bad!!!",
	}))
	require.NoError(t, err)
	require.Equal(t, "Invalid search cursor", wsWorkflowError(t, garbage).Message)

	// A cursor minted for another task is rejected, never silently rebound.
	foreign := searchcursor.Encode(
		searchcursor.Binding{TaskID: "other-task", ActiveSessionID: "sess-a", Query: "needle"},
		searchcursor.Cursor{Bucket: 0, Key: "2026-01-01 00:00:00.000000", ID: "m-1"},
	)
	mismatch, err := h.wsSearchMessages(ctx, wsWorkflowRequest(t, ws.ActionMessageSearch, map[string]any{
		"task_id": "task-x", "active_session_id": "sess-a", "query": "needle", "cursor": foreign,
	}))
	require.NoError(t, err)
	require.Equal(t, "Invalid search cursor", wsWorkflowError(t, mismatch).Message)
	require.Zero(t, repo.taskSearchCalls)
}

func TestWSSearchTaskMessagesBlankQueryShortCircuits(t *testing.T) {
	repo := &messageListRepo{}
	h := newMessageListHandlers(t, repo)

	resp, err := h.wsSearchMessages(context.Background(),
		wsWorkflowRequest(t, ws.ActionMessageSearch, map[string]any{
			"task_id": "task-x", "active_session_id": "sess-a", "query": "   ",
		}))
	require.NoError(t, err)
	var out dto.SearchMessagesResponse
	wsWorkflowResponse(t, resp, &out)
	require.Zero(t, out.Total)
	require.Zero(t, repo.taskSearchCalls)
}
