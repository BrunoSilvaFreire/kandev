package orchestrator

import (
	"context"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/possiblequestion"
)

// onSessionStateChangedForPossibleQuestion evaluates whether a session settling
// into WAITING_FOR_INPUT ended with a trailing question/decision cue in agent
// prose, without an active structured clarification, workflow move, or step completion.
// When leaving WAITING_FOR_INPUT, any advisory possible-question flag is cleared.
func (s *Service) onSessionStateChangedForPossibleQuestion(
	ctx context.Context,
	taskID, sessionID string,
	oldState, nextState models.TaskSessionState,
	session *models.TaskSession,
) {
	if sessionID == "" || session == nil {
		return
	}
	switch {
	case nextState == models.TaskSessionStateWaitingForInput && oldState != models.TaskSessionStateWaitingForInput:
		s.checkAndSetPossibleQuestion(ctx, taskID, sessionID, session)
	case oldState == models.TaskSessionStateWaitingForInput && nextState != models.TaskSessionStateWaitingForInput:
		s.clearPossibleQuestion(ctx, sessionID, session)
	}
}

//nolint:cyclop,funlen,gocognit,nestif // Sequential fail-closed guards for one advisory projection.
func (s *Service) checkAndSetPossibleQuestion(
	ctx context.Context,
	taskID, sessionID string,
	session *models.TaskSession,
) {
	// 1. If session already has a pending step completion or is parked, it's not waiting for user clarification.
	// A cleared bag entry persists as a nil key, so presence alone is not a
	// pending signal: use the canonical loader that requires a decoded signal.
	if session.Metadata != nil {
		if _, ok := models.LoadPendingStepSignal(session.Metadata); ok {
			s.clearPossibleQuestion(ctx, sessionID, session)
			return
		}
		if parked, ok := session.Metadata["parked_on_background_work"].(bool); ok && parked {
			s.clearPossibleQuestion(ctx, sessionID, session)
			return
		}
	}

	// 2. Check if the task has a pending workflow move or is parked on background work. Fail closed on error.
	if taskID != "" {
		task, err := s.repo.GetTask(ctx, taskID)
		if err != nil {
			s.clearPossibleQuestion(ctx, sessionID, session)
			return
		}
		if task != nil && task.Metadata != nil {
			if _, ok := task.Metadata[models.MetaKeyWorkflowMovePending]; ok {
				s.clearPossibleQuestion(ctx, sessionID, session)
				return
			}
			if parked, ok := task.Metadata["parked_on_background_work"].(bool); ok && parked {
				s.clearPossibleQuestion(ctx, sessionID, session)
				return
			}
		}
	}

	// 3. Check if the session has an active structured clarification or permission pending. Fail closed on error or when the lookup is unavailable:
	// an unreadable pending-action state must never be treated as "no pending action".
	if reader, ok := s.repo.(pendingActionsBySessionIDsReader); ok {
		actions, err := reader.GetPendingActionsBySessionIDs(ctx, []string{sessionID})
		if err != nil {
			s.clearPossibleQuestion(ctx, sessionID, session)
			return
		}
		if action, exists := actions[sessionID]; exists && action != "" {
			s.clearPossibleQuestion(ctx, sessionID, session)
			return
		}
	} else {
		s.clearPossibleQuestion(ctx, sessionID, session)
		return
	}

	// 4. Inspect messages from the session to find the last agent prose message. Fail closed on error.
	msgs, err := s.repo.ListMessages(ctx, sessionID)
	if err != nil || len(msgs) == 0 {
		s.clearPossibleQuestion(ctx, sessionID, session)
		return
	}

	var lastAgentMsg *models.Message
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		if m == nil {
			continue
		}
		// If user sent a message after the agent, this turn has user input.
		if m.AuthorType == models.MessageAuthorUser {
			break
		}
		if m.AuthorType == models.MessageAuthorAgent && (m.Type == models.MessageTypeMessage || m.Type == "") {
			lastAgentMsg = m
			break
		}
	}

	if lastAgentMsg == nil || lastAgentMsg.Content == "" {
		s.clearPossibleQuestion(ctx, sessionID, session)
		return
	}

	// Structured inputs are handled through pending actions.
	if lastAgentMsg.RequestsInput {
		s.clearPossibleQuestion(ctx, sessionID, session)
		return
	}

	// Check if this turn issued an in-turn move or step completion call that didn't fail
	if turnAppliedMoveOrStepCompletion(msgs, lastAgentMsg.TurnID) {
		s.clearPossibleQuestion(ctx, sessionID, session)
		return
	}

	detected, _ := possiblequestion.Detect(lastAgentMsg.Content)
	if !detected {
		s.clearPossibleQuestion(ctx, sessionID, session)
		return
	}

	// Detected: persist advisory flag with session and turn identity.
	hint := map[string]interface{}{
		"active":     true,
		"session_id": sessionID,
		"turn_id":    lastAgentMsg.TurnID,
	}
	if session.Metadata == nil {
		session.Metadata = make(map[string]interface{})
	}
	session.Metadata[models.SessionMetaKeyPossibleQuestion] = hint
	if err := s.persistPossibleQuestionMetadata(ctx, sessionID, hint); err != nil {
		s.logger.Debug("failed to persist possible_question metadata key",
			zap.String("session_id", sessionID),
			zap.Error(err))
	}
}

// advisorySessionMetadataWriter persists a derived, clearable metadata key
// without bumping the session's authoritative state-change timestamp. The
// possible_question hint is advisory, so its write must not reorder or
// desynchronize the published session state timestamp.
type advisorySessionMetadataWriter interface {
	SetSessionAdvisoryMetadataKey(ctx context.Context, sessionID, key string, value interface{}) error
}

type pendingActionsBySessionIDsReader interface {
	GetPendingActionsBySessionIDs(ctx context.Context, sessionIDs []string) (map[string]models.TaskPendingAction, error)
}

func (s *Service) persistPossibleQuestionMetadata(ctx context.Context, sessionID string, value interface{}) error {
	if writer, ok := s.repo.(advisorySessionMetadataWriter); ok {
		return writer.SetSessionAdvisoryMetadataKey(ctx, sessionID, models.SessionMetaKeyPossibleQuestion, value)
	}
	return s.repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyPossibleQuestion, value)
}

//nolint:cyclop,gocognit,nestif // Single pass over one turn's tool-call/execute records.
func turnAppliedMoveOrStepCompletion(msgs []*models.Message, turnID string) bool {
	failedCalls := make(map[string]bool)
	for _, m := range msgs {
		if m == nil || m.TurnID != turnID {
			continue
		}
		if m.Type == models.MessageTypeToolExecute && m.Metadata != nil {
			callID, _ := m.Metadata["tool_call_id"].(string)
			isErr, _ := m.Metadata["is_error"].(bool)
			status, _ := m.Metadata["status"].(string)
			if isErr || status == agentEventError || m.Metadata["error"] != nil {
				if callID != "" {
					failedCalls[callID] = true
				}
			}
		}
	}

	for _, m := range msgs {
		if m == nil || m.TurnID != turnID {
			continue
		}
		if m.Type == models.MessageTypeToolCall || m.Type == models.MessageTypeToolExecute {
			if m.Metadata == nil {
				continue
			}
			toolName, _ := m.Metadata["tool_name"].(string)
			if toolName == "" {
				toolName, _ = m.Metadata["name"].(string)
			}
			if toolName == "" {
				toolName, _ = m.Metadata["tool"].(string)
			}
			isMoveOrComplete := strings.Contains(toolName, "move_task") || strings.Contains(toolName, "step_complete")
			if !isMoveOrComplete {
				continue
			}
			callID, _ := m.Metadata["tool_call_id"].(string)
			if callID != "" && failedCalls[callID] {
				continue
			}
			isErr, _ := m.Metadata["is_error"].(bool)
			status, _ := m.Metadata["status"].(string)
			if isErr || status == agentEventError || m.Metadata["error"] != nil {
				continue
			}
			return true
		}
	}
	return false
}

func (s *Service) clearPossibleQuestion(
	ctx context.Context,
	sessionID string,
	session *models.TaskSession,
) {
	if session != nil && session.Metadata != nil {
		delete(session.Metadata, models.SessionMetaKeyPossibleQuestion)
	}
	if sessionID != "" {
		if err := s.persistPossibleQuestionMetadata(ctx, sessionID, false); err != nil {
			s.logger.Debug("failed to clear possible_question metadata key",
				zap.String("session_id", sessionID),
				zap.Error(err))
		}
	}
}
