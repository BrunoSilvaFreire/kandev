package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/kandev/kandev/internal/steptelemetry"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type getTaskCompletionGateRequest struct {
	TaskID string `json:"task_id"`
}

type setTaskCompletionCriteriaRequest struct {
	TaskID           string                           `json:"task_id"`
	ExpectedRevision int64                            `json:"expected_revision"`
	PlanRevisionID   string                           `json:"plan_revision_id,omitempty"`
	Criteria         []models.TaskCompletionCriterion `json:"criteria"`
	SenderSessionID  string                           `json:"sender_session_id,omitempty"`
}

type verifyTaskCompletionCriterionRequest struct {
	TaskID           string                        `json:"task_id"`
	ExpectedRevision int64                         `json:"expected_revision"`
	CriterionID      string                        `json:"criterion_id"`
	Evidence         models.TaskCompletionEvidence `json:"evidence"`
	SenderSessionID  string                        `json:"sender_session_id,omitempty"`
}

type enrollTaskPlanIncrementsRequest struct {
	TaskID           string                                `json:"task_id"`
	ExpectedRevision int64                                 `json:"expected_revision"`
	PlanRevisionID   string                                `json:"plan_revision_id"`
	Increments       []models.TaskPlanIncrementDeclaration `json:"increments"`
	SenderSessionID  string                                `json:"sender_session_id,omitempty"`
}

type recordPlanMinorApprovalRequest struct {
	TaskID           string                                `json:"task_id"`
	ExpectedRevision int64                                 `json:"expected_revision"`
	PlanRevisionID   string                                `json:"plan_revision_id,omitempty"`
	Summary          string                                `json:"summary"`
	Attestation      string                                `json:"attestation"`
	Increments       []models.TaskPlanIncrementDeclaration `json:"increments"`
	SenderSessionID  string                                `json:"sender_session_id,omitempty"`
}

type manageTaskCompletionCriteriaRequest struct {
	Operation        string                                `json:"operation,omitempty"`
	Action           string                                `json:"action,omitempty"`
	TaskID           string                                `json:"task_id"`
	ExpectedRevision int64                                 `json:"expected_revision"`
	PlanRevisionID   string                                `json:"plan_revision_id,omitempty"`
	Criteria         []models.TaskCompletionCriterion      `json:"criteria,omitempty"`
	Increments       []models.TaskPlanIncrementDeclaration `json:"increments,omitempty"`
	CriterionID      string                                `json:"criterion_id,omitempty"`
	Evidence         *models.TaskCompletionEvidence        `json:"evidence,omitempty"`
	Summary          string                                `json:"summary,omitempty"`
	Attestation      string                                `json:"attestation,omitempty"`
	SenderSessionID  string                                `json:"sender_session_id,omitempty"`
}

func (h *Handlers) registerCompletionGateHandlers(d *guardedMCPDispatcher) {
	d.RegisterFunc(ws.ActionMCPGetTaskCompletionGate, h.handleGetTaskCompletionGate)
	d.RegisterFunc(ws.ActionMCPSetTaskCompletionCriteria, h.handleSetTaskCompletionCriteria)
	d.RegisterFunc(ws.ActionMCPEnrollTaskPlanIncrements, h.handleEnrollTaskPlanIncrements)
	d.RegisterFunc(ws.ActionMCPRecordPlanMinorApproval, h.handleRecordPlanMinorApproval)
	d.RegisterFunc(ws.ActionMCPVerifyTaskCompletionCriterion, h.handleVerifyTaskCompletionCriterion)
	d.RegisterFunc(ws.ActionMCPManageTaskCompletionCriteria, h.handleManageTaskCompletionCriteria)
}

func (h *Handlers) handleGetTaskCompletionGate(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	if h.taskSvc == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "task service is not configured", nil)
	}
	var req getTaskCompletionGateRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if strings.TrimSpace(req.TaskID) == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}
	snapshot, err := h.taskSvc.GetTaskCompletionGate(ctx, req.TaskID)
	if err != nil {
		return h.handleCompletionGateOpError(msg, err)
	}
	// A transition that failed after its move or signal was accepted is
	// reported here so a later turn can read why the task did not advance.
	failure, _ := h.taskSvc.GetTransitionFailure(ctx, req.TaskID)
	return ws.NewResponse(msg.ID, msg.Action, completionGateWithTransitionFailure{
		TaskCompletionGateSnapshot: snapshot, TransitionFailure: failure,
	})
}

type completionGateWithTransitionFailure struct {
	*models.TaskCompletionGateSnapshot
	TransitionFailure *service.TransitionFailureRecord `json:"transition_failure,omitempty"`
}

func (h *Handlers) handleSetTaskCompletionCriteria(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	if h.taskSvc == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "task service is not configured", nil)
	}
	var req setTaskCompletionCriteriaRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if strings.TrimSpace(req.TaskID) == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}
	if len(req.Criteria) == 0 {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "criteria cannot be empty", nil)
	}
	attribution := steptelemetry.Attribution{
		Trigger:   steptelemetry.TriggerMCPMove,
		ActorKind: steptelemetry.ActorAgent,
		ActorID:   req.SenderSessionID,
		SessionID: req.SenderSessionID,
	}
	opCtx := steptelemetry.WithAttribution(ctx, attribution)
	snapshot, err := h.taskSvc.SetTaskCompletionCriteria(opCtx, req.TaskID, service.SetTaskCompletionCriteriaRequest{
		ExpectedRevision: req.ExpectedRevision,
		PlanRevisionID:   strings.TrimSpace(req.PlanRevisionID),
		Criteria:         req.Criteria,
	})
	if err != nil {
		return h.handleCompletionGateOpError(msg, err)
	}
	return ws.NewResponse(msg.ID, msg.Action, snapshot)
}

func (h *Handlers) handleEnrollTaskPlanIncrements(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	if h.taskSvc == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "task service is not configured", nil)
	}
	var req enrollTaskPlanIncrementsRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if strings.TrimSpace(req.TaskID) == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}
	if strings.TrimSpace(req.PlanRevisionID) == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "plan_revision_id is required", nil)
	}
	if len(req.Increments) == 0 {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "increments cannot be empty", nil)
	}
	attribution := steptelemetry.Attribution{
		Trigger:   steptelemetry.TriggerMCPMove,
		ActorKind: steptelemetry.ActorAgent,
		ActorID:   req.SenderSessionID,
		SessionID: req.SenderSessionID,
	}
	opCtx := steptelemetry.WithAttribution(ctx, attribution)
	snapshot, err := h.taskSvc.EnrollTaskPlanIncrements(opCtx, req.TaskID, models.EnrollTaskPlanIncrementsRequest{
		PlanRevisionID:   strings.TrimSpace(req.PlanRevisionID),
		ExpectedRevision: req.ExpectedRevision,
		Increments:       req.Increments,
	})
	if err != nil {
		return h.handleCompletionGateOpError(msg, err)
	}
	return ws.NewResponse(msg.ID, msg.Action, snapshot)
}

func (h *Handlers) handleRecordPlanMinorApproval(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	if h.taskSvc == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "task service is not configured", nil)
	}
	var req recordPlanMinorApprovalRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if strings.TrimSpace(req.TaskID) == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}
	if strings.TrimSpace(req.Summary) == "" || strings.TrimSpace(req.Attestation) == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "summary and attestation are required", nil)
	}
	if len(req.Increments) == 0 {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "increments cannot be empty", nil)
	}
	attribution := steptelemetry.Attribution{
		Trigger:   steptelemetry.TriggerMCPMove,
		ActorKind: steptelemetry.ActorAgent,
		ActorID:   req.SenderSessionID,
		SessionID: req.SenderSessionID,
	}
	opCtx := steptelemetry.WithAttribution(ctx, attribution)
	snapshot, err := h.taskSvc.RecordPlanMinorApproval(opCtx, req.TaskID, models.RecordPlanMinorApprovalRequest{
		ExpectedRevision:       req.ExpectedRevision,
		ExpectedPlanRevisionID: strings.TrimSpace(req.PlanRevisionID),
		Summary:                req.Summary,
		Attestation:            req.Attestation,
		Increments:             req.Increments,
	})
	if err != nil {
		return h.handleCompletionGateOpError(msg, err)
	}
	return ws.NewResponse(msg.ID, msg.Action, snapshot)
}

func (h *Handlers) handleVerifyTaskCompletionCriterion(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	if h.taskSvc == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "task service is not configured", nil)
	}
	var req verifyTaskCompletionCriterionRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if strings.TrimSpace(req.TaskID) == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}
	if strings.TrimSpace(req.CriterionID) == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "criterion_id is required", nil)
	}
	if strings.TrimSpace(req.Evidence.Summary) == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "evidence.summary is required", nil)
	}
	attribution := steptelemetry.Attribution{
		Trigger:   steptelemetry.TriggerMCPMove,
		ActorKind: steptelemetry.ActorAgent,
		ActorID:   req.SenderSessionID,
		SessionID: req.SenderSessionID,
	}
	opCtx := steptelemetry.WithAttribution(ctx, attribution)
	snapshot, err := h.taskSvc.VerifyTaskCompletionCriterion(opCtx, req.TaskID, service.VerifyTaskCompletionCriterionRequest{
		ExpectedRevision: req.ExpectedRevision,
		CriterionID:      strings.TrimSpace(req.CriterionID),
		Evidence:         req.Evidence,
	})
	if err != nil {
		return h.handleCompletionGateOpError(msg, err)
	}
	return ws.NewResponse(msg.ID, msg.Action, snapshot)
}

func (h *Handlers) handleManageTaskCompletionCriteria(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req manageTaskCompletionCriteriaRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	op := strings.ToLower(strings.TrimSpace(req.Operation))
	if op == "" {
		op = strings.ToLower(strings.TrimSpace(req.Action))
	}
	switch op {
	case "get", "read":
		return h.handleGetTaskCompletionGate(ctx, msg)
	case "set", "register":
		return h.handleSetTaskCompletionCriteria(ctx, msg)
	case "enroll":
		raw, err := json.Marshal(enrollTaskPlanIncrementsRequest{
			TaskID:           req.TaskID,
			ExpectedRevision: req.ExpectedRevision,
			PlanRevisionID:   req.PlanRevisionID,
			Increments:       req.Increments,
			SenderSessionID:  req.SenderSessionID,
		})
		if err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid enroll payload: "+err.Error(), nil)
		}
		forwardMsg := *msg
		forwardMsg.Payload = raw
		return h.handleEnrollTaskPlanIncrements(ctx, &forwardMsg)
	case "verify":
		var evidence models.TaskCompletionEvidence
		if req.Evidence != nil {
			evidence = *req.Evidence
		}
		raw, err := json.Marshal(verifyTaskCompletionCriterionRequest{
			TaskID:           req.TaskID,
			ExpectedRevision: req.ExpectedRevision,
			CriterionID:      req.CriterionID,
			Evidence:         evidence,
			SenderSessionID:  req.SenderSessionID,
		})
		if err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid verify payload: "+err.Error(), nil)
		}
		forwardMsg := *msg
		forwardMsg.Payload = raw
		return h.handleVerifyTaskCompletionCriterion(ctx, &forwardMsg)
	case "minor_approval", "minor":
		raw, err := json.Marshal(recordPlanMinorApprovalRequest{
			TaskID:           req.TaskID,
			ExpectedRevision: req.ExpectedRevision,
			PlanRevisionID:   req.PlanRevisionID,
			Summary:          req.Summary,
			Attestation:      req.Attestation,
			Increments:       req.Increments,
			SenderSessionID:  req.SenderSessionID,
		})
		if err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid minor approval payload: "+err.Error(), nil)
		}
		forwardMsg := *msg
		forwardMsg.Payload = raw
		return h.handleRecordPlanMinorApproval(ctx, &forwardMsg)
	default:
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "invalid operation: must be get, set, enroll, verify, or minor_approval", nil)
	}
}

func (h *Handlers) handleCompletionGateOpError(msg *ws.Message, err error) (*ws.Message, error) {
	switch {
	case errors.Is(err, repoerrors.ErrTaskCompletionCriteriaConflict):
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeConflict, "task completion criteria conflict: expected_revision is stale", nil)
	case errors.Is(err, repoerrors.ErrTaskCompletionEvidenceChanged):
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeConflict, "task completion evidence conflict: criterion changed or subject is stale", nil)
	case errors.Is(err, repoerrors.ErrTaskCompletionHumanConfirmationRequired):
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeConflict, "task completion human confirmation required: weakening or removing unmet criteria requires human confirmation with reason", nil)
	case errors.Is(err, repoerrors.ErrTaskPlanMinorApprovalNotBaselinePrefix):
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeConflict, "minor_approval_not_baseline_prefix: the revision must be the prior approved plan plus exactly one trailing '## Architect clarifications (advisory)' section; request human approval", nil)
	case errors.Is(err, repoerrors.ErrTaskPlanMinorApprovalCriteriaChanged):
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeConflict, "minor_approval_criteria_changed: criterion IDs, descriptions, and evidence subjects must be unchanged; request human approval", nil)
	case errors.Is(err, repoerrors.ErrTaskPlanMinorApprovalActorNotPlanOwner):
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "minor_approval_actor_not_plan_owner: only the active session on the baseline's workflow step may self-approve", nil)
	case errors.Is(err, repoerrors.ErrTaskPlanMinorApprovalNoHumanBaseline):
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeConflict, "minor_approval_no_human_baseline: a human-approved plan baseline is required before a minor clarification", nil)
	case errors.Is(err, repoerrors.ErrTaskPlanMinorApprovalStaleHead):
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeConflict, "minor_approval_stale_head: the plan head changed; re-read the current revision and retry", nil)
	case errors.Is(err, repoerrors.ErrTaskCompletionGateBlocked):
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeTaskCompletionGateBlocked, "task completion requirements are not satisfied", nil)
	case errors.Is(err, repoerrors.ErrTaskNotFound):
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "task not found", nil)
	case service.IsForbidden(err):
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "access denied to task", nil)
	default:
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
	}
}
