package orchestrator

import (
	"context"
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
	outcome models.RoutingOutcome,
	reason models.RoutingReason,
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
		Outcome:                   outcome,
		Reason:                    reason,
		WorkflowStepTransitionID:  latestEntryTransitionID(ctx, s, taskID, entryIDs...),
		CorrelationID:             fmt.Sprintf("workflow-route:%s:%s:%s:%s", taskID, step.ID, entryIdentity, destination.ID),
		CreatedAt:                 time.Now().UTC(),
	}
	if err := recorder.RecordSessionRoute(ctx, route); err != nil {
		s.logger.Warn("failed to record workflow session route decision",
			zap.String("task_id", taskID),
			zap.String("step_id", step.ID),
			zap.String("outcome", string(outcome)),
			zap.String("reason", string(reason)),
			zap.Error(err))
	}
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

// classifySwitchRoutingOutcome reports the committed outcome/reason for a
// profile-switch routing path from the sessions it actually selected: the
// current session (reuse in place), a validated reusable candidate, or a
// freshly created destination.
func classifySwitchRoutingOutcome(
	reasonForNew models.RoutingReason,
	configuredStartPolicy models.WorkflowProfileSessionStartPolicy,
	startPolicy models.WorkflowProfileSessionStartPolicy,
	currentSession *models.TaskSession,
	validatedExisting *models.TaskSession,
	destination *models.TaskSession,
) (models.RoutingOutcome, models.RoutingReason) {
	if destination == nil {
		return models.RoutingOutcomeDeclined, reasonForNew
	}
	if currentSession != nil && destination.ID == currentSession.ID {
		return models.RoutingOutcomeReused, models.RoutingReasonReusedCurrentSession
	}
	if validatedExisting != nil && destination.ID == validatedExisting.ID {
		return models.RoutingOutcomeReused, models.RoutingReasonReusedExisting
	}
	if configuredStartPolicy == models.WorkflowProfileSessionStartPolicyNew || startPolicy == models.WorkflowProfileSessionStartPolicyNew {
		return models.RoutingOutcomeCreated, models.RoutingReasonForcedNewPolicy
	}
	return models.RoutingOutcomeCreated, reasonForNew
}
