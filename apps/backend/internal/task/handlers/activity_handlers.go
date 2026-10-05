package handlers

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/task/activitycursor"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// wsListTaskActivity returns one bounded page of the merged, chronological
// task activity stream. `step_id` narrows to step visits; `cursor` is the
// opaque exclusive keyset cursor.
func (h *TaskHandlers) wsListTaskActivity(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID string `json:"task_id"`
		StepID string `json:"step_id"`
		Limit  int    `json:"limit"`
		Cursor string `json:"cursor"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}
	var before *time.Time
	if strings.TrimSpace(req.Cursor) != "" {
		at, err := activitycursor.Decode(req.Cursor, req.TaskID)
		if err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "Invalid activity cursor", nil)
		}
		before = &at
	}
	events, hasMore, err := h.service.ListTaskActivity(ctx, req.TaskID, models.TaskActivityFilter{
		StepID: req.StepID,
		Before: before,
		Limit:  req.Limit,
	})
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to list task activity", nil)
	}
	out := make([]dto.TaskActivityEventDTO, 0, len(events))
	for _, event := range events {
		out = append(out, dto.TaskActivityEventFromModel(event))
	}
	resp := dto.TaskActivityListResponse{Events: out, HasMore: hasMore}
	if hasMore && len(events) > 0 {
		resp.NextCursor = activitycursor.Encode(req.TaskID, events[len(events)-1].OccurredAt)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

// wsTaskTransitionCounts returns the task-local committed invocation count per
// directed step pair for the header transition summary.
func (h *TaskHandlers) wsTaskTransitionCounts(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}
	counts, err := h.service.TaskTransitionSummary(ctx, req.TaskID)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to load transition counts", nil)
	}
	out := make([]dto.StepTransitionCountDTO, 0, len(counts.Counts))
	for _, count := range counts.Counts {
		out = append(out, dto.StepTransitionCountDTO{
			FromStepID: count.FromStepID, ToStepID: count.ToStepID, Count: count.Count,
		})
	}
	visits := make([]dto.StepVisitSummaryDTO, 0, len(counts.Visits))
	for _, visit := range counts.Visits {
		sessions := make([]dto.StepVisitSessionDTO, 0, len(visit.Sessions))
		for _, session := range visit.Sessions {
			sessions = append(sessions, dto.StepVisitSessionDTO{
				SessionID:      session.SessionID,
				AgentProfileID: session.AgentProfileID,
				TransitionID:   session.TransitionID,
				OccurredAt:     session.OccurredAt,
			})
		}
		visits = append(visits, dto.StepVisitSummaryDTO{
			StepID: visit.StepID, Count: visit.Count, Sessions: sessions,
		})
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.TaskTransitionCountsResponse{Counts: out, Visits: visits})
}
