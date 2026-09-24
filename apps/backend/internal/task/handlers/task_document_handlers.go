package handlers

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/planws"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// wsListTaskDocumentsCatalog returns the metadata-only merged catalog of the
// authoritative Plan and general task documents, grouped by the producing
// session of each entry's latest revision.
func (h *TaskHandlers) wsListTaskDocumentsCatalog(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req planws.TaskIDRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if h.documentCatalog == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "document catalog is unavailable", nil)
	}
	groups, err := h.documentCatalog.CatalogGrouped(ctx, req.TaskID)
	if err != nil {
		return documentError(msg, err, "Failed to list task documents")
	}
	out := make([]dto.TaskDocumentCatalogGroupDTO, 0, len(groups))
	for _, group := range groups {
		out = append(out, dto.DocumentCatalogGroupFromService(group))
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{"groups": out})
}

// wsGetTaskDocument returns a selected document's content plus the latest
// revision's provenance. The synthetic Plan key resolves through the plan
// service so the Plan panel keeps one authoritative source.
func (h *TaskHandlers) wsGetTaskDocument(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID string `json:"task_id"`
		Key    string `json:"key"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}
	if req.Key == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "key is required", nil)
	}

	if req.Key == service.PlanDocumentKey {
		return h.taskDocumentPlanDetail(ctx, msg, req.TaskID)
	}
	if h.documentService == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "document service is unavailable", nil)
	}
	doc, err := h.documentService.GetDocument(ctx, req.TaskID, req.Key)
	if err != nil {
		if errors.Is(err, service.ErrDocumentNotFound) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "Document not found", nil)
		}
		return documentError(msg, err, "Failed to get task document")
	}
	latest, err := h.latestDocumentRevision(ctx, req.TaskID, req.Key)
	if err != nil {
		return documentError(msg, err, "Failed to get task document")
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.DocumentDetailFromModels(doc, latest))
}

func (h *TaskHandlers) taskDocumentPlanDetail(ctx context.Context, msg *ws.Message, taskID string) (*ws.Message, error) {
	if h.planService == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "plan service is unavailable", nil)
	}
	plan, err := h.planService.GetPlan(ctx, taskID)
	if err != nil {
		return documentError(msg, err, "Failed to get task plan")
	}
	if plan == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "Plan not found", nil)
	}
	revisions, err := h.planService.ListRevisions(ctx, taskID)
	if err != nil {
		return documentError(msg, err, "Failed to get task plan")
	}
	var latest *models.TaskPlanRevision
	if len(revisions) > 0 {
		latest = revisions[0]
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.PlanDetailFromModel(plan, latest))
}

// latestDocumentRevision returns the newest revision for a general document,
// or nil when none exist.
func (h *TaskHandlers) latestDocumentRevision(ctx context.Context, taskID, key string) (*models.TaskDocumentRevision, error) {
	revisions, err := h.documentService.ListRevisions(ctx, taskID, key, 1)
	if err != nil {
		return nil, err
	}
	if len(revisions) == 0 {
		return nil, nil
	}
	return revisions[0], nil
}

// wsListTaskDocumentRevisions returns revision metadata (newest-first) without
// bodies. The synthetic Plan key resolves through the plan service.
func (h *TaskHandlers) wsListTaskDocumentRevisions(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID string `json:"task_id"`
		Key    string `json:"key"`
		Limit  int    `json:"limit"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}
	if req.Key == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "key is required", nil)
	}
	if req.Key == service.PlanDocumentKey {
		return h.taskDocumentPlanRevisions(ctx, msg, req.TaskID, req.Limit)
	}
	if h.documentService == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "document service is unavailable", nil)
	}
	revisions, err := h.documentService.ListRevisions(ctx, req.TaskID, req.Key, req.Limit)
	if err != nil {
		return documentError(msg, err, "Failed to list task document revisions")
	}
	out := make([]*dto.TaskDocumentRevisionDTO, 0, len(revisions))
	for _, rev := range revisions {
		out = append(out, dto.TaskDocumentRevisionFromModel(rev, false))
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{"revisions": out})
}

func (h *TaskHandlers) taskDocumentPlanRevisions(ctx context.Context, msg *ws.Message, taskID string, limit int) (*ws.Message, error) {
	if h.planService == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "plan service is unavailable", nil)
	}
	revisions, err := h.planService.ListRevisions(ctx, taskID)
	if err != nil {
		return documentError(msg, err, "Failed to list task plan revisions")
	}
	if limit > 0 && len(revisions) > limit {
		revisions = revisions[:limit]
	}
	out := make([]*dto.TaskDocumentRevisionDTO, 0, len(revisions))
	for _, rev := range revisions {
		out = append(out, dto.TaskDocumentRevisionFromPlanRevision(rev, false))
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{"revisions": out})
}

func documentError(msg *ws.Message, err error, message string) (*ws.Message, error) {
	return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, message+": "+err.Error(), nil)
}
