package orchestrator

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/clarification"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"go.uber.org/zap"
)

// continuationRecoveryBundleType is the message type the inbox and chat render
// as a clarification bundle.
const continuationRecoveryBundleType = "clarification_request"

// continuationRecoveryStampRemover removes one task metadata key only when its
// nested stamp still matches. It is implemented by the sqlite/Postgres
// repository; the fallback keeps test doubles useful.
type continuationRecoveryStampRemover interface {
	RemoveTaskMetadataKeyIfStamp(ctx context.Context, taskID, key, expectedStamp string) (bool, error)
}

// ResolveContinuationRecovery applies a retry/continue decision to the paused
// automatic continuation named by stamp. A missing or mismatched stamp is a
// no-op, so a second answer or a later step entry cannot re-run a settled case.
func (s *Service) ResolveContinuationRecovery(ctx context.Context, taskID, stamp, decision string) error {
	if strings.TrimSpace(taskID) == "" || strings.TrimSpace(stamp) == "" {
		return nil
	}
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return fmt.Errorf("continuation recovery: load task: %w", err)
	}
	if task == nil {
		return nil
	}
	record := models.PendingContinuationRecord(task.Metadata)
	if record == nil || record.Stamp != stamp {
		return nil
	}
	session, err := s.repo.GetTaskSession(ctx, record.SourceSessionID)
	if err != nil || session == nil {
		return fmt.Errorf("continuation recovery: load session %s", record.SourceSessionID)
	}
	switch decision {
	case clarification.ContinuationRecoveryDecisionContinue:
		return s.continueContinuationWithoutHandoff(ctx, taskID, session, record)
	case clarification.ContinuationRecoveryDecisionRetry:
		return s.retryContinuationRecovery(ctx, taskID, session, record)
	default:
		return nil
	}
}

// enterContinuationRecovery persists the pending record, publishes the metadata
// update, and creates the system-authored recovery bundle. It replaces any
// existing record (single-slot by construction) with a fresh stamp. It returns
// false when the pause could not be persisted, so the caller can fall back to
// sending the entry prompt instead of dropping it.
func (s *Service) enterContinuationRecovery(
	ctx context.Context,
	taskID string,
	session *models.TaskSession,
	step *wfmodels.WorkflowStep,
	entryPrompt string,
	entryIDs []int64,
	cse, reason, extractedHandoff string,
) bool {
	stepID := ""
	if step != nil {
		stepID = step.ID
	}
	record := &models.PendingContinuation{
		Stamp:            uuid.New().String(),
		StepID:           stepID,
		EntryID:          firstEntryID(entryIDs),
		Case:             cse,
		SourceSessionID:  session.ID,
		Reason:           reason,
		EntryPrompt:      entryPrompt,
		ExtractedHandoff: extractedHandoff,
	}
	if err := s.repo.SetTaskMetadataKey(ctx, taskID, models.MetaKeyPendingContinuation, record); err != nil {
		s.logger.Error("continuation recovery: failed to persist pending record",
			zap.String("task_id", taskID),
			zap.String("session_id", session.ID),
			zap.String("reason", reason),
			zap.Error(err))
		return false
	}
	s.publishTaskUpdatedByID(ctx, taskID)
	s.createContinuationRecoveryBundle(ctx, taskID, session.ID, record)
	return true
}

// createContinuationRecoveryBundle emits the fixed single-question bundle on
// the source session. It has no agent waiter: the resolver forwards the chosen
// decision to ResolveContinuationRecovery.
func (s *Service) createContinuationRecoveryBundle(
	ctx context.Context,
	taskID, sessionID string,
	record *models.PendingContinuation,
) {
	if s.messageCreator == nil {
		s.logger.Warn("continuation recovery: message creator unavailable",
			zap.String("task_id", taskID))
		return
	}
	question := clarification.ContinuationRecoveryQuestion()
	options := make([]interface{}, len(question.Options))
	for i, opt := range question.Options {
		options[i] = map[string]interface{}{
			"option_id":   opt.ID,
			"label":       opt.Label,
			"description": opt.Description,
		}
	}
	metadata := map[string]interface{}{
		"pending_id":  uuid.New().String(),
		"question_id": question.ID,
		"question": map[string]interface{}{
			"id":      question.ID,
			"title":   question.Title,
			"prompt":  question.Prompt,
			"options": options,
		},
		"question_index": 0,
		"question_total": 1,
		"status":         "pending",
		"continuation_recovery": &clarification.ContinuationRecoveryMeta{
			Stamp:  record.Stamp,
			Case:   record.Case,
			Reason: record.Reason,
		},
	}
	if err := s.messageCreator.CreateSessionMessage(
		ctx, taskID, question.Prompt, sessionID, continuationRecoveryBundleType,
		s.getActiveTurnID(sessionID), metadata, true,
	); err != nil {
		s.logger.Error("continuation recovery: failed to create bundle",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(err))
	}
}

// clearPendingContinuation removes the pending record only when its stamp still
// matches, so a settled case is never cleared by a stale answer.
func (s *Service) clearPendingContinuation(ctx context.Context, taskID, stamp string) bool {
	if strings.TrimSpace(stamp) == "" {
		return false
	}
	if remover, ok := s.repo.(continuationRecoveryStampRemover); ok {
		removed, err := remover.RemoveTaskMetadataKeyIfStamp(ctx, taskID, models.MetaKeyPendingContinuation, stamp)
		if err != nil {
			s.logger.Warn("continuation recovery: failed to clear pending record",
				zap.String("task_id", taskID), zap.Error(err))
			return false
		}
		return removed
	}
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil || task == nil {
		return false
	}
	record := models.PendingContinuationRecord(task.Metadata)
	if record == nil || record.Stamp != stamp {
		return false
	}
	if _, err := s.repo.RemoveTaskMetadataKey(ctx, taskID, models.MetaKeyPendingContinuation); err != nil {
		s.logger.Warn("continuation recovery: failed to clear pending record",
			zap.String("task_id", taskID), zap.Error(err))
		return false
	}
	return true
}

// continueContinuationWithoutHandoff sends the paused entry prompt to the
// unreset session, which is today's behavior. The record is cleared only after
// a successful send, so a failed send leaves the pending state intact for a
// retry.
func (s *Service) continueContinuationWithoutHandoff(
	ctx context.Context,
	taskID string,
	session *models.TaskSession,
	record *models.PendingContinuation,
) error {
	if err := s.dispatchContinuationPrompt(ctx, taskID, session, record, record.EntryPrompt); err != nil {
		return err
	}
	s.clearPendingContinuation(ctx, taskID, record.Stamp)
	s.publishTaskUpdatedByID(ctx, taskID)
	s.logger.Info("continuation.auto",
		zap.String("outcome", "recovery_continue"),
		zap.String("task_id", taskID),
		zap.String("session_id", session.ID))
	return nil
}

// retryContinuationRecovery re-runs the paused case. A reset failure reuses the
// already-extracted text; an extraction failure extracts again. A repeated
// failure replaces the pending record with a new stamp and bundle, so the user
// can decide again.
func (s *Service) retryContinuationRecovery(
	ctx context.Context,
	taskID string,
	session *models.TaskSession,
	record *models.PendingContinuation,
) error {
	step := s.workflowStepForContinuation(ctx, record.StepID)
	handoff := record.ExtractedHandoff
	if strings.TrimSpace(handoff) == "" {
		extractCtx, cancel := context.WithTimeout(ctx, resumeHandoffExtractionTimeout)
		extracted, extractErr := s.extractResumeHandoff(extractCtx, session.ID)
		cancel()
		if extractErr != nil {
			s.repauseContinuationRecovery(ctx, taskID, session, step, record,
				models.PendingContinuationReasonExtractionFailed, "")
			return nil
		}
		handoff = extracted
	}
	stepName := ""
	if step != nil {
		stepName = step.Name
	}
	if _, resetErr := s.resetAgentContextWithError(ctx, taskID, session, stepName); resetErr != nil {
		s.repauseContinuationRecovery(ctx, taskID, session, step, record,
			models.PendingContinuationReasonResetFailed, handoff)
		return nil
	}
	s.markIdleAfterReset(ctx, taskID, session.ID, session, step, false)
	s.createContextResetMessage(ctx, taskID, session.ID)
	composed := composeResumeHandoffPrompt(handoff, record.EntryPrompt)
	if err := s.dispatchContinuationPrompt(ctx, taskID, session, record, composed); err != nil {
		return err
	}
	s.clearPendingContinuation(ctx, taskID, record.Stamp)
	s.publishTaskUpdatedByID(ctx, taskID)
	s.logger.Info("continuation.auto",
		zap.String("outcome", "recovery_retry"),
		zap.String("task_id", taskID),
		zap.String("session_id", session.ID))
	return nil
}

// dispatchContinuationPrompt re-runs the paused entry's prompt through the
// normal step-entry dispatch so the queued move hand-off message and the step
// handoff carry are merged exactly as they would have been on the original
// entry. When the step is unavailable (deleted), it falls back to a direct
// prompt.
func (s *Service) dispatchContinuationPrompt(
	ctx context.Context,
	taskID string,
	session *models.TaskSession,
	record *models.PendingContinuation,
	prompt string,
) error {
	step := s.workflowStepForContinuation(ctx, record.StepID)
	if step != nil && s.agentManager != nil {
		isPassthrough := s.agentManager.IsPassthroughSession(ctx, session.ID)
		hasPlanMode := s.resolveStepPlanMode(ctx, session, step, isPassthrough)
		if err := s.autoStartStepPrompt(ctx, taskID, session, step, prompt, hasPlanMode, true, newStepHandoffOnce()); err != nil {
			return fmt.Errorf("continuation recovery: dispatch prompt: %w", err)
		}
		return nil
	}
	if strings.TrimSpace(prompt) == "" {
		return nil
	}
	return s.sendContinuationEntryPrompt(ctx, taskID, session.ID, prompt)
}

// repauseContinuationRecovery replaces the pending record with a fresh stamp,
// case, reason, and any extracted text, then re-asks.
func (s *Service) repauseContinuationRecovery(
	ctx context.Context,
	taskID string,
	session *models.TaskSession,
	step *wfmodels.WorkflowStep,
	record *models.PendingContinuation,
	reason, extractedHandoff string,
) {
	s.enterContinuationRecovery(ctx, taskID, session, step, record.EntryPrompt, nil,
		record.Case, reason, extractedHandoff)
}

// clearPendingContinuationOnEntry drops any pending record when a new step
// entry begins for the task, so a record that was never answered cannot shadow
// the new entry.
func (s *Service) clearPendingContinuationOnEntry(ctx context.Context, taskID string) {
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil || task == nil {
		return
	}
	record := models.PendingContinuationRecord(task.Metadata)
	if record == nil {
		return
	}
	// TODO: also finalize/dismiss the stale continuation_recovery bundle; it stays pending in the inbox and a later answer is a silent no-op that reports success.
	if s.clearPendingContinuation(ctx, taskID, record.Stamp) {
		s.publishTaskUpdatedByID(ctx, taskID)
	}
}

func (s *Service) sendContinuationEntryPrompt(ctx context.Context, taskID, sessionID, prompt string) error {
	if s.messageCreator != nil {
		meta := NewUserMessageMeta().WithPlanMode(false)
		if err := s.messageCreator.CreateUserMessage(
			ctx, taskID, prompt, sessionID, s.getActiveTurnID(sessionID), meta.ToMap(),
		); err != nil {
			s.logger.Warn("continuation recovery: failed to record user message",
				zap.String("session_id", sessionID), zap.Error(err))
		}
	}
	if _, err := s.PromptTask(ctx, taskID, sessionID, prompt, "", false, nil, true); err != nil {
		return fmt.Errorf("continuation recovery: dispatch prompt: %w", err)
	}
	return nil
}

func (s *Service) workflowStepForContinuation(ctx context.Context, stepID string) *wfmodels.WorkflowStep {
	if s.workflowStepGetter == nil || strings.TrimSpace(stepID) == "" {
		return nil
	}
	step, err := s.workflowStepGetter.GetStep(ctx, stepID)
	if err != nil || step == nil {
		return nil
	}
	return step
}

func firstEntryID(entryIDs []int64) string {
	if len(entryIDs) == 0 || entryIDs[0] == 0 {
		return ""
	}
	return strconv.FormatInt(entryIDs[0], 10)
}
