// Package transitionfailure owns the one public description of a declined or
// failed workflow transition (REQ-TASKS-TRANSITION-DIAGNOSTICS-001). Adapters
// serialize a Failure; they never reclassify the error.
package transitionfailure

import (
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowmove "github.com/kandev/kandev/internal/workflow/move"
)

// Top-level codes. The pre-existing ones keep their wire value.
const (
	CodeGateBlocked               = "task_completion_gate_blocked"
	CodeCriteriaConflict          = "task_completion_criteria_conflict"
	CodeHumanConfirmationRequired = "task_completion_human_confirmation_required"
	CodeActiveSession             = "task_move_active_session"
	CodeArchived                  = "task_move_archived"
	CodeDifferentWorkspace        = "task_move_different_workspace"
	CodeWorkflowStep              = "task_move_workflow_step"
	CodeWIPLimit                  = "task_move_wip_limit"
	CodePending                   = "task_move_pending"
	CodeWorkflowChangeConflict    = "workflow_change_conflict"
	CodeInvalid                   = "task_move_invalid"
	CodeTransitionUnknown         = "task_transition_unknown"
	CodeNotFound                  = "task_move_not_found"
	CodeStale                     = "task_move_stale"
	CodeInternal                  = "task_transition_internal_failure"
)

// Actionability separates what the caller can repair from operator faults.
const (
	ActionabilityUser   = "user"
	ActionabilitySystem = "system"
)

// Rule identifiers name the failed guard inside a Reason.
const (
	RulePlanStale             = "completion_gate.plan_stale"
	RuleCriterionUnverified   = "completion_gate.criterion_unverified"
	RuleEvidenceStale         = "completion_gate.evidence_stale"
	RuleCriteriaChanged       = "completion_gate.criteria_changed"
	RuleHumanConfirmation     = "completion_gate.human_confirmation_required"
	RuleArchived              = "move.archived"
	RuleActiveSession         = "move.active_session"
	RuleDifferentWorkspace    = "move.different_workspace"
	RuleStepNotInWorkflow     = "move.step_not_in_workflow"
	RuleWIPLimit              = "move.wip_limit"
	RulePendingMove           = "move.pending_move"
	RuleTaskChanged           = "move.task_changed"
	RuleEntryOptions          = "move.entry_options"
	RuleEntryTarget           = "move.entry_target_unavailable"
	RuleInvalidRequest        = "move.invalid_request"
	RuleUnknownTransition     = "transition.unknown"
	RuleTargetNotFound        = "move.target_not_found"
	RuleInternal              = "system.internal"
	ctaOpenPlan               = "open_plan"
	ctaOpenSession            = "open_session"
	ctaRetry                  = "retry"
	gateBlockerUnverified     = "unverified"
	gateBlockerPlanCriterion  = "plan"
	remediationGatePlanStale  = "Request approval for the current plan revision, then re-enroll its increments."
	remediationGateUnverified = "Verify these increments with evidence."
)

// CTA is a safe next action the UI may offer. Target is an identifier, never
// free text.
type CTA struct {
	Kind   string `json:"kind"`
	Target string `json:"target,omitempty"`
}

// Reason is one failed rule with what was observed and what repairs it.
type Reason struct {
	Rule        string `json:"rule"`
	CriterionID string `json:"criterion_id,omitempty"`
	StepID      string `json:"step_id,omitempty"`
	Description string `json:"description,omitempty"`
	Observed    string `json:"observed,omitempty"`
	Required    string `json:"required,omitempty"`
	Action      string `json:"action"`
	CTA         *CTA   `json:"cta,omitempty"`
}

// Gate carries the completion-gate identities a gate refusal was decided on.
type Gate struct {
	Revision                 int64  `json:"gate_revision"`
	EnrolledPlanRevisionID   string `json:"enrolled_plan_revision_id,omitempty"`
	EnrolledPlanWriteVersion string `json:"enrolled_plan_write_version,omitempty"`
	CurrentPlanWriteVersion  string `json:"current_plan_write_version,omitempty"`
	PlanStale                bool   `json:"plan_stale"`
}

// Failure is the envelope every caller receives for the same failure.
type Failure struct {
	Code          string   `json:"code"`
	Message       string   `json:"message"`
	Actionability string   `json:"actionability"`
	Reasons       []Reason `json:"reasons"`
	Remediation   string   `json:"remediation,omitempty"`
	IncidentID    string   `json:"incident_id,omitempty"`
	Gate          *Gate    `json:"gate,omitempty"`
}

// ruleError is a typed cause: a guard names its rule and keeps its message.
type ruleError struct {
	rule     string
	msg      string
	observed string
}

func (e *ruleError) Error() string { return e.msg }

// Rule returns an error whose text is exactly msg and whose failed rule the
// classifier can read.
func Rule(rule, msg string) error { return &ruleError{rule: rule, msg: msg} }

// RuleObserved is Rule with the observed state attached.
func RuleObserved(rule, msg, observed string) error {
	return &ruleError{rule: rule, msg: msg, observed: observed}
}

// coded is implemented by validation errors that already carry a stable,
// client-visible code.
type coded interface{ TransitionFailureCode() string }

// Input is the fresh task-scoped state the classifier may describe.
type Input struct {
	// Gate is the gate authority's own snapshot; nil when unreadable.
	Gate *models.TaskCompletionGateSnapshot
	// NotVisible marks a missing or unauthorized object. Both read the same.
	NotVisible bool
}

type staticFailure struct {
	code, message, rule, required, action string
	cta                                   string
}

var ruleFailures = map[string]staticFailure{
	RuleArchived: {CodeArchived, "Archived tasks cannot be moved.", RuleArchived,
		"task is not archived", "Unarchive the task, then move it.", ""},
	RuleActiveSession: {CodeActiveSession, "The task has a running session.", RuleActiveSession,
		"no session is starting or running", "Wait for the session to finish or stop it, then move the task.", ctaOpenSession},
	RuleDifferentWorkspace: {CodeDifferentWorkspace, "The target workflow belongs to a different workspace.", RuleDifferentWorkspace,
		"target workflow in the task's workspace", "Choose a workflow in the task's workspace.", ""},
	RuleStepNotInWorkflow: {CodeWorkflowStep, "The target step does not belong to the target workflow.", RuleStepNotInWorkflow,
		"a step of the target workflow", "List the workflow's steps and choose one of them.", ""},
	RuleInvalidRequest: {CodeInvalid, "The move request is not valid.", RuleInvalidRequest,
		"a well-formed move request", "Correct the request parameters and call again.", ""},
	RuleTargetNotFound: {CodeNotFound, "The task or its target was not found.", RuleTargetNotFound,
		"an accessible task, workflow, and step", "Check the identifiers and your access, then try again.", ""},
	RuleUnknownTransition: {CodeTransitionUnknown, "The current step has no transition with that name.", RuleUnknownTransition,
		"a transition declared on the current step", "Use one of the current step's declared transitions.", ""},
}

// Classify builds the envelope for err. It never returns raw error text for
// an unrecognized cause.
func Classify(err error, in Input) Failure {
	if in.NotVisible {
		return single(CodeNotFound, "The task or its target was not found.", Reason{
			Rule: RuleTargetNotFound, Required: "an accessible task, workflow, and step",
			Action: "Check the identifiers and your access, then try again.",
		})
	}
	var rule *ruleError
	if errors.As(err, &rule) {
		if known, ok := ruleFailures[rule.rule]; ok {
			return fromStatic(known, rule.observed)
		}
	}
	if failure, ok := classifyGate(err, in.Gate); ok {
		return failure
	}
	if failure, ok := classifySentinel(err); ok {
		return failure
	}
	var c coded
	if errors.As(err, &c) {
		return single(c.TransitionFailureCode(), "The workflow change is not valid.", Reason{
			Rule: RuleInvalidRequest, Observed: c.TransitionFailureCode(),
			Action: "Correct the workflow change and submit it again.",
		})
	}
	return internal()
}

func fromStatic(known staticFailure, observed string) Failure {
	reason := Reason{Rule: known.rule, Observed: observed, Required: known.required, Action: known.action}
	if known.cta != "" {
		reason.CTA = &CTA{Kind: known.cta}
	}
	return single(known.code, known.message, reason)
}

func single(code, message string, reason Reason) Failure {
	return Failure{
		Code: code, Message: message, Actionability: ActionabilityUser,
		Reasons: []Reason{reason}, Remediation: reason.Action,
	}
}

func internal() Failure {
	failure := single(CodeInternal, "The transition failed because of an internal error.", Reason{
		Rule:   RuleInternal,
		Action: "Retry once. If it fails again, report the incident ID to the operator.",
		CTA:    &CTA{Kind: ctaRetry},
	})
	failure.Actionability = ActionabilitySystem
	failure.IncidentID = uuid.NewString()
	return failure
}

//nolint:cyclop // One flat table of typed causes.
func classifySentinel(err error) (Failure, bool) {
	switch {
	case errors.Is(err, wfmodels.ErrWIPLimitExceeded):
		return single(CodeWIPLimit, "The target step is at its work-in-progress limit.", Reason{
			Rule: RuleWIPLimit, Observed: "step at capacity", Required: "a free slot in the target step",
			Action: "Move a task out of the target step or raise its limit, then try again.",
		}), true
	case errors.Is(err, workflowmove.ErrMoveConflict), errors.Is(err, workflowmove.ErrPendingMoveActiveSession):
		return single(CodePending, "Another move for this task is already pending.", Reason{
			Rule: RulePendingMove, Observed: "a pending move exists", Required: "no pending move",
			Action: "Wait for the pending move to apply at the end of the turn, then try again.",
		}), true
	case errors.Is(err, repoerrors.ErrWorkflowChangeConflict), errors.Is(err, repoerrors.ErrTaskVersionConflict),
		errors.Is(err, repoerrors.ErrStepChanged):
		code := CodeStale
		if errors.Is(err, repoerrors.ErrWorkflowChangeConflict) {
			code = CodeWorkflowChangeConflict
		}
		return single(code, "The task changed since this move was prepared.", Reason{
			Rule: RuleTaskChanged, Observed: "task state is newer than the request", Required: "the current task state",
			Action: "Reload the task and repeat the move against its current step.", CTA: &CTA{Kind: ctaRetry},
		}), true
	case errors.Is(err, workflowmove.ErrEntryTargetUnavailable):
		return single(CodeInvalid, "The target step has no session to receive the entry options.", Reason{
			Rule: RuleEntryTarget, Required: "a session or auto-start on the target step",
			Action: "Move without entry options, or choose a step that starts or reuses a session.",
		}), true
	case errors.Is(err, workflowmove.ErrConflictingInstructions), errors.Is(err, workflowmove.ErrEntryOptionsRequireStepChange),
		errors.Is(err, workflowmove.ErrEntryOptionsUnsupported), errors.Is(err, workflowmove.ErrPermanentPendingMoveMismatch):
		return single(CodeInvalid, "The move's entry options are not valid.", Reason{
			Rule: RuleEntryOptions, Observed: err.Error(), Required: "entry options valid for a step change",
			Action: "Correct or remove the entry options and move again.",
		}), true
	case errors.Is(err, repoerrors.ErrTaskNotFound):
		return Classify(nil, Input{NotVisible: true}), true
	}
	return Failure{}, false
}

func classifyGate(err error, gate *models.TaskCompletionGateSnapshot) (Failure, bool) {
	switch {
	case errors.Is(err, repoerrors.ErrTaskCompletionCriteriaConflict):
		return single(CodeCriteriaConflict, "The completion criteria changed since they were read.", Reason{
			Rule: RuleCriteriaChanged, Required: "the current gate revision",
			Action: "Read the completion gate again and repeat the request with its revision.", CTA: &CTA{Kind: ctaRetry},
		}), true
	case errors.Is(err, repoerrors.ErrTaskCompletionHumanConfirmationRequired):
		return single(CodeHumanConfirmationRequired, "A human must confirm this completion change.", Reason{
			Rule: RuleHumanConfirmation, Required: "a human confirmation for the current gate revision",
			Action: "Ask a human to confirm the change in the task UI.",
		}), true
	case errors.Is(err, repoerrors.ErrTaskCompletionGateBlocked):
		return gateBlocked(gate), true
	}
	return Failure{}, false
}

// gateBlocked lists every blocker in gate order: the stale plan first, then
// criteria as the gate authority returned them.
func gateBlocked(gate *models.TaskCompletionGateSnapshot) Failure {
	failure := Failure{
		Code: CodeGateBlocked, Message: "Task completion requirements are not satisfied.",
		Actionability: ActionabilityUser, Remediation: remediationGateUnverified,
	}
	if gate == nil {
		failure.Reasons = []Reason{{
			Rule: RuleCriterionUnverified, Required: "every completion criterion verified",
			Action: "Read the completion gate, then verify each unmet criterion with evidence.",
		}}
		return failure
	}
	failure.Gate = &Gate{
		Revision: gate.Revision, EnrolledPlanRevisionID: gate.PlanRevisionID,
		EnrolledPlanWriteVersion: gate.PlanWriteVersion, CurrentPlanWriteVersion: gate.CurrentPlanWriteVersion,
		PlanStale: gate.PlanStale,
	}
	if gate.PlanStale {
		failure.Remediation = remediationGatePlanStale
		failure.Reasons = append(failure.Reasons, Reason{
			Rule:        RulePlanStale,
			Description: "The task plan was modified after its increments were enrolled.",
			Observed:    "plan write version " + gate.CurrentPlanWriteVersion,
			Required:    "enrolled plan write version " + gate.PlanWriteVersion,
			Action:      remediationGatePlanStale,
			CTA:         &CTA{Kind: ctaOpenPlan},
		})
	}
	descriptions := make(map[string]models.TaskCompletionCriterion, len(gate.Criteria))
	for _, criterion := range gate.Criteria {
		descriptions[criterion.ID] = criterion
	}
	for _, blocker := range gate.Blockers {
		if blocker.CriterionID == gateBlockerPlanCriterion && strings.HasPrefix(blocker.Reason, "plan_revision_stale") {
			continue
		}
		criterion := descriptions[blocker.CriterionID]
		reason := Reason{
			Rule: RuleCriterionUnverified, CriterionID: blocker.CriterionID, Description: criterion.Description,
			Observed: fmt.Sprintf("verified revision %d", criterion.VerifiedRevision),
			Required: fmt.Sprintf("verified revision %d with evidence", criterion.CriterionRevision),
			Action:   remediationGateUnverified,
		}
		if blocker.Reason != gateBlockerUnverified {
			reason.Rule = RuleEvidenceStale
			reason.Observed = "evidence no longer matches its subject"
		}
		failure.Reasons = append(failure.Reasons, reason)
	}
	if len(failure.Reasons) == 0 {
		failure.Reasons = []Reason{{
			Rule: RuleCriterionUnverified, Required: "enrolled completion criteria",
			Action: "Enroll the approved plan's increments, then verify them with evidence.",
		}}
	}
	return failure
}

// Details is the envelope as the generic detail map WS and MCP errors carry.
// "code" and "error_code" keep the keys older clients already read.
func (f Failure) Details() map[string]interface{} {
	details := map[string]interface{}{
		"code": f.Code, "error_code": f.Code, "message": f.Message,
		"actionability": f.Actionability, "reasons": f.Reasons,
	}
	if f.Remediation != "" {
		details["remediation"] = f.Remediation
	}
	if f.IncidentID != "" {
		details["incident_id"] = f.IncidentID
	}
	if f.Gate != nil {
		details["gate"] = f.Gate
	}
	return details
}
