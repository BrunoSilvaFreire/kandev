package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// workflowRouteLedgerRecorder is the optional repository capability backing the
// durable task session-route ledger. Repositories that do not implement it
// (test fakes, legacy adapters) simply skip ledger recording.
type workflowRouteLedgerRecorder interface {
	RecordSessionRoute(context.Context, *models.TaskSessionRoute) error
}

// recordWorkflowRouteDecision appends one durable routing decision. It is a
// best-effort side effect: a persistence failure is logged and never changes
// the routing outcome the runtime already committed to. The correlation id
// includes the destination session, so a retry of the same decision is a no-op
// while a corrected destination (for example after a terminal race) is
// recorded as its own row.
func (s *Service) recordWorkflowRouteDecision(
	ctx context.Context,
	taskID string,
	sourceSession *models.TaskSession,
	destination *models.TaskSession,
	step *wfmodels.WorkflowStep,
	decision workflowRouteDecision,
	startPolicy string,
	endPolicy string,
	entryIDs ...int64,
) {
	if s == nil || s.repo == nil || step == nil || destination == nil || taskID == "" {
		return
	}
	recorder, ok := s.repo.(workflowRouteLedgerRecorder)
	if !ok {
		return
	}
	var sourceSessionID *string
	if sourceSession != nil && sourceSession.ID != "" {
		value := sourceSession.ID
		sourceSessionID = &value
	}
	destinationID := destination.ID
	entryIdentity := s.workflowEntryIdentity(ctx, taskID, entryIDs...)
	route := &models.TaskSessionRoute{
		TaskID:                    taskID,
		DestinationWorkflowStepID: step.ID,
		SourceSessionID:           sourceSessionID,
		DestinationSessionID:      &destinationID,
		AgentProfileID:            destination.AgentProfileID,
		StartPolicy:               startPolicy,
		EndPolicy:                 endPolicy,
		Outcome:                   decision.Outcome,
		Reason:                    decision.Reason,
		DecisionDetail:            nullableDecisionDetail(decision.Detail),
		WorkflowStepTransitionID:  latestEntryTransitionID(ctx, s, taskID, entryIDs...),
		CorrelationID:             fmt.Sprintf("workflow-route:%s:%s:%s:%s", taskID, step.ID, entryIdentity, destination.ID),
		CreatedAt:                 time.Now().UTC(),
	}
	if err := recorder.RecordSessionRoute(ctx, route); err != nil {
		s.logger.Warn("failed to record workflow session route decision",
			zap.String("task_id", taskID),
			zap.String("step_id", step.ID),
			zap.String("outcome", string(decision.Outcome)),
			zap.String("reason", string(decision.Reason)),
			zap.Error(err))
	}
}

// nullableDecisionDetail keeps an empty detail out of the payload so legacy
// rows and detail-less decisions share one NULL representation.
func nullableDecisionDetail(detail string) *string {
	if detail == "" {
		return nil
	}
	return &detail
}

// latestEntryTransitionID resolves the immutable step-transition ledger id that
// anchors this decision, when one is available. It never guesses.
func latestEntryTransitionID(ctx context.Context, s *Service, taskID string, entryIDs ...int64) *int64 {
	if len(entryIDs) > 0 && entryIDs[0] > 0 {
		value := entryIDs[0]
		return &value
	}
	reader, ok := s.repo.(workflowStepTransitionReader)
	if !ok {
		return nil
	}
	transitionID, err := reader.GetLatestTaskStepTransitionID(ctx, taskID)
	if err != nil || transitionID <= 0 {
		return nil
	}
	return &transitionID
}

// workflowRouteDecision is the single typed result of one routing decision.
// Both the execution path and the durable route ledger consume the same value,
// so the reason recorded is exactly the decision the runtime made rather than a
// reverse-classification reconstructed from session identities afterwards.
type workflowRouteDecision struct {
	Outcome models.RoutingOutcome
	Reason  models.RoutingReason
	// Detail is optional closed-key JSON the route ledger stores verbatim.
	Detail string
}

// routeDecisionDetail is the closed set of decision inputs stored in
// decision_detail. Every field is optional; an all-empty detail serializes to
// an empty string and is stored as NULL.
type routeDecisionDetail struct {
	CandidateSessionID string `json:"candidate_session_id,omitempty"`
	CandidateState     string `json:"candidate_state,omitempty"`
	RequiredModel      string `json:"required_model,omitempty"`
	CandidateModel     string `json:"candidate_model,omitempty"`
	StartPolicy        string `json:"start_policy,omitempty"`
	Fitness            string `json:"fitness,omitempty"`
	SourceSessionID    string `json:"source_session_id,omitempty"`
}

func marshalDecisionDetail(detail routeDecisionDetail) string {
	encoded, err := json.Marshal(detail)
	if err != nil || string(encoded) == "{}" {
		return ""
	}
	return string(encoded)
}

// The constructors below name each committed routing decision once. Callers
// return the decision from the same branch that selects the recipient, so the
// ledger cannot drift from what actually happened.

func decisionReusedCurrentSession() workflowRouteDecision {
	return workflowRouteDecision{Outcome: models.RoutingOutcomeReused, Reason: models.RoutingReasonReusedCurrentSession}
}

func decisionReusedExisting(candidateSessionID, candidateState string) workflowRouteDecision {
	return workflowRouteDecision{
		Outcome: models.RoutingOutcomeReused,
		Reason:  models.RoutingReasonReusedExisting,
		Detail:  marshalDecisionDetail(routeDecisionDetail{CandidateSessionID: candidateSessionID, CandidateState: candidateState}),
	}
}

func decisionStepPrimary(candidateSessionID, fitness string) workflowRouteDecision {
	return workflowRouteDecision{
		Outcome: models.RoutingOutcomeReused,
		Reason:  models.RoutingReasonStepPrimary,
		Detail:  marshalDecisionDetail(routeDecisionDetail{CandidateSessionID: candidateSessionID, Fitness: fitness}),
	}
}

func decisionUnavailableHandoff(sourceSessionID, fitness string) workflowRouteDecision {
	return workflowRouteDecision{
		Outcome: models.RoutingOutcomeCreated,
		Reason:  models.RoutingReasonUnavailableHandoff,
		Detail:  marshalDecisionDetail(routeDecisionDetail{SourceSessionID: sourceSessionID, Fitness: fitness}),
	}
}

func decisionReusedExplicitTarget() workflowRouteDecision {
	return workflowRouteDecision{Outcome: models.RoutingOutcomeReused, Reason: models.RoutingReasonExplicitTarget}
}

func decisionCreatedExplicitTarget() workflowRouteDecision {
	return workflowRouteDecision{Outcome: models.RoutingOutcomeCreated, Reason: models.RoutingReasonExplicitTarget}
}

func decisionForcedNewPolicy(startPolicy string) workflowRouteDecision {
	return workflowRouteDecision{
		Outcome: models.RoutingOutcomeCreated,
		Reason:  models.RoutingReasonForcedNewPolicy,
		Detail:  marshalDecisionDetail(routeDecisionDetail{StartPolicy: startPolicy}),
	}
}

func decisionNoReusableCandidate() workflowRouteDecision {
	return workflowRouteDecision{Outcome: models.RoutingOutcomeCreated, Reason: models.RoutingReasonNoReusableCandidate}
}

func decisionExactModelIncompatibility(requiredModel, candidateModel string) workflowRouteDecision {
	return workflowRouteDecision{
		Outcome: models.RoutingOutcomeCreated,
		Reason:  models.RoutingReasonExactModelIncompatibility,
		Detail:  marshalDecisionDetail(routeDecisionDetail{RequiredModel: requiredModel, CandidateModel: candidateModel}),
	}
}

func decisionSelectedCandidateTerminal(candidateSessionID, candidateState string) workflowRouteDecision {
	return workflowRouteDecision{
		Outcome: models.RoutingOutcomeCreated,
		Reason:  models.RoutingReasonSelectedCandidateTerminal,
		Detail:  marshalDecisionDetail(routeDecisionDetail{CandidateSessionID: candidateSessionID, CandidateState: candidateState}),
	}
}
