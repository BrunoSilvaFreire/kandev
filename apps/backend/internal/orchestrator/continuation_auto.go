package orchestrator

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"go.uber.org/zap"
)

// resumeHandoffCacheExpiry mirrors apps/web/lib/usage/efficiency.ts
// CACHE_EXPIRY_MS (1h). Providers do not expose a prompt-cache TTL, so this is
// the shared heuristic; keep both in sync.
const resumeHandoffCacheExpiry = time.Hour

// resumeHandoffExtractionTimeout bounds the automatic extraction so a slow
// utility call cannot hold the step-entry path indefinitely.
const resumeHandoffExtractionTimeout = 3 * time.Minute

// autoResumeOutcome reports what the automatic continuation step did: it did
// not apply, it applied and returned a handoff, or it paused the entry (D10)
// and nothing must be sent.
type autoResumeOutcome int

const (
	autoResumeNotApplied autoResumeOutcome = iota
	autoResumeApplied
	autoResumePaused
)

// sessionCacheCold reports whether the session has recorded usage and its newest
// usage event is older than resumeHandoffCacheExpiry. Unknown (no provider or no
// events) is never cold, matching the web cacheStatus heuristic.
func (s *Service) sessionCacheCold(ctx context.Context, sessionID string) bool {
	if s.sessionUsageTotals == nil || sessionID == "" {
		return false
	}
	totals, err := s.sessionUsageTotals(ctx, sessionID)
	if err != nil || totals == nil || totals.EventCount <= 0 || totals.LastEventAt == nil {
		return false
	}
	return time.Since(*totals.LastEventAt) >= resumeHandoffCacheExpiry
}

// shouldAutoResumeWithHandoff reports whether step entry should extract a
// handoff and reset the reused session's context before sending the entry
// prompt. It requires the feature flag, an idle ACP session, a live extraction
// runner, no explicit reset action, and a cold cache.
func (s *Service) shouldAutoResumeWithHandoff(
	ctx context.Context,
	session *models.TaskSession,
	step *wfmodels.WorkflowStep,
	isPassthrough bool,
) bool {
	if !s.config.ColdCacheAutoHandoff || s.sessionlessRunner == nil {
		return false
	}
	if session == nil || step == nil || isPassthrough {
		return false
	}
	if step.HasOnEnterAction(wfmodels.OnEnterResetAgentContext) {
		return false
	}
	// A cold session is idle by definition; a running turn must not be reset.
	if session.State != models.TaskSessionStateWaitingForInput {
		return false
	}
	return s.sessionCacheCold(ctx, session.ID)
}

// applyAutoResumeHandoff extracts a facts-only handoff, resets the live ACP
// context, and returns the handoff so the caller can compose it into the entry
// prompt. A successful extraction that fails to reset, or a failed/empty
// extraction, pauses the entry (D10) instead of falling back silently: the
// caller sends nothing and the user chooses retry or continue.
func (s *Service) applyAutoResumeHandoff(
	ctx context.Context,
	taskID string,
	session *models.TaskSession,
	step *wfmodels.WorkflowStep,
	entryPrompt string,
	entryIDs ...int64,
) (autoResumeOutcome, string) {
	extractCtx, cancel := context.WithTimeout(ctx, resumeHandoffExtractionTimeout)
	defer cancel()
	handoff, err := s.extractResumeHandoff(extractCtx, session.ID)
	if err != nil {
		s.logger.Warn("continuation.auto: extraction failed, pausing step entry",
			zap.String("outcome", "paused_extraction_failed"),
			zap.String("task_id", taskID),
			zap.String("session_id", session.ID),
			zap.String("step_id", step.ID),
			zap.Error(err))
		if !s.enterContinuationRecovery(ctx, taskID, session, step, entryPrompt, entryIDs,
			models.PendingContinuationCaseCold, models.PendingContinuationReasonExtractionFailed, "") {
			// The pause could not be persisted, so there is no recovery record
			// for the user to answer. Fall back to sending the entry prompt
			// (today's behavior) rather than dropping it.
			return autoResumeNotApplied, ""
		}
		return autoResumePaused, ""
	}
	if _, err := s.resetAgentContextWithError(ctx, taskID, session, step.Name); err != nil {
		s.logger.Warn("continuation.auto: context reset failed, pausing step entry",
			zap.String("outcome", "paused_reset_failed"),
			zap.String("task_id", taskID),
			zap.String("session_id", session.ID),
			zap.String("step_id", step.ID),
			zap.Error(err))
		if !s.enterContinuationRecovery(ctx, taskID, session, step, entryPrompt, entryIDs,
			models.PendingContinuationCaseCold, models.PendingContinuationReasonResetFailed, handoff) {
			return autoResumeNotApplied, ""
		}
		return autoResumePaused, ""
	}
	s.markIdleAfterReset(ctx, taskID, session.ID, session, step, false)
	s.createContextResetMessage(ctx, taskID, session.ID)
	s.logger.Info("continuation.auto",
		zap.String("outcome", "resume_handoff"),
		zap.String("task_id", taskID),
		zap.String("session_id", session.ID),
		zap.String("step_id", step.ID))
	return autoResumeApplied, handoff
}
