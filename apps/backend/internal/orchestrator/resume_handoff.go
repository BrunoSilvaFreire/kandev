package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

const (
	// ResumeHandoffAgentID is the builtin utility agent for extracting factual resume handoffs.
	ResumeHandoffAgentID = "builtin-extract-resume-handoff"
)

var (
	// ErrResumeHandoffEmpty is returned when resume handoff extraction produces no text.
	ErrResumeHandoffEmpty = errors.New("resume handoff extraction produced no text")

	// ErrSessionNotEligible is returned when a session is not in WAITING_FOR_INPUT state or has no live agent execution.
	ErrSessionNotEligible = errors.New("session not eligible for resume with handoff")

	// ErrExtractionFailed is returned when the sessionless utility prompt execution fails.
	ErrExtractionFailed = errors.New("failed to extract resume handoff")

	// ErrProviderUnavailable is returned when the session's provider quota is exhausted.
	ErrProviderUnavailable = errors.New("provider quota exhausted")
)

// ResumeHandoffResult is the outcome of a ResumeWithHandoff operation.
type ResumeHandoffResult struct {
	Handoff string `json:"handoff"`
	Prompt  string `json:"prompt"`
	Sent    bool   `json:"sent"`
}

// formatHandoffTranscript converts a sequence of task session messages into a
// transcript string for the extraction utility model, matching the web client format.
func formatHandoffTranscript(messages []*models.Message) string {
	var parts []string
	for _, m := range messages {
		if m == nil {
			continue
		}
		if m.Type == "" || m.Type == models.MessageTypeMessage || m.Type == models.MessageTypeContent {
			role := "Agent"
			if m.AuthorType == models.MessageAuthorUser {
				role = "User"
			}
			parts = append(parts, fmt.Sprintf("%s: %s", role, m.Content))
		}
	}
	return strings.Join(parts, "\n\n")
}

// composeResumeHandoffPrompt formats the final prompt sent to the fresh session context.
// When instructions is non-empty, it appends an "## Additional instructions" section per D3.
func composeResumeHandoffPrompt(handoff, instructions string) string {
	trimmedInstructions := strings.TrimSpace(instructions)
	if trimmedInstructions == "" {
		return handoff
	}
	return fmt.Sprintf("%s\n\n## Additional instructions\n\n%s", handoff, trimmedInstructions)
}

// extractResumeHandoff loads the transcript of a session and invokes the builtin
// extract-resume-handoff utility agent.
func (s *Service) extractResumeHandoff(ctx context.Context, sessionID string) (string, error) {
	if s.sessionlessRunner == nil {
		return "", errors.New("sessionless utility runner not configured")
	}
	messages, err := s.repo.ListMessages(ctx, sessionID)
	if err != nil {
		return "", fmt.Errorf("failed to list session messages: %w", err)
	}
	if len(messages) == 0 {
		return "", ErrResumeHandoffEmpty
	}
	transcript := formatHandoffTranscript(messages)
	if strings.TrimSpace(transcript) == "" {
		return "", ErrResumeHandoffEmpty
	}
	text, err := s.sessionlessRunner(ctx, ResumeHandoffAgentID, transcript)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrExtractionFailed, err)
	}
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "", ErrResumeHandoffEmpty
	}
	return trimmed, nil
}

func (s *Service) validateResumeHandoffEligibility(ctx context.Context, session *models.TaskSession) error {
	if session.State != models.TaskSessionStateWaitingForInput || session.IsPassthrough {
		return ErrSessionNotEligible
	}
	hasRunning, hasErr := s.repo.HasExecutorRunningRow(ctx, session.ID)
	if hasErr != nil || !hasRunning {
		return ErrSessionNotEligible
	}

	if s.profileExecutionResolver != nil {
		targetProfileID := session.ExecutionProfileID
		if targetProfileID == "" {
			targetProfileID = session.AgentProfileID
		}
		if targetProfileID != "" && s.profileExecutionResolver.ProfileQuotaExhausted(ctx, targetProfileID) {
			return ErrProviderUnavailable
		}
	}
	return nil
}

// ResumeWithHandoff extracts a facts-only handoff from the session transcript,
// resets the live agent's context, and dispatches the composed handoff and
// optional additional instructions as the first prompt of the fresh context.
func (s *Service) ResumeWithHandoff(ctx context.Context, sessionID, instructions string) (ResumeHandoffResult, error) {
	if err := s.authorizeSession(ctx, sessionID); err != nil {
		return ResumeHandoffResult{}, err
	}

	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return ResumeHandoffResult{}, fmt.Errorf("session not found: %w", err)
	}

	if err := s.validateResumeHandoffEligibility(ctx, session); err != nil {
		return ResumeHandoffResult{}, err
	}

	handoff, err := s.extractResumeHandoff(ctx, sessionID)
	if err != nil {
		s.logger.Warn("resume with handoff: extraction failed",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return ResumeHandoffResult{}, ErrExtractionFailed
	}

	if err := s.ResetAgentContext(ctx, sessionID); err != nil {
		return ResumeHandoffResult{}, fmt.Errorf("failed to reset agent context: %w", err)
	}

	composedPrompt := composeResumeHandoffPrompt(handoff, instructions)

	// Record the user message before dispatch so it lands in the transcript at
	// the prompt boundary rather than after the agent's first reply events.
	if s.messageCreator != nil {
		meta := NewUserMessageMeta().WithPlanMode(false)
		if msgErr := s.messageCreator.CreateUserMessage(ctx, session.TaskID, composedPrompt, sessionID, s.getActiveTurnID(sessionID), meta.ToMap()); msgErr != nil {
			s.logger.Error("resume with handoff: failed to create user message",
				zap.String("session_id", sessionID),
				zap.Error(msgErr))
		}
	}

	_, promptErr := s.PromptTask(ctx, session.TaskID, sessionID, composedPrompt, "", false, nil, true)
	if promptErr != nil {
		s.logger.Warn("resume with handoff: failed to dispatch prompt to agent after reset",
			zap.String("session_id", sessionID),
			zap.Error(promptErr))
		return ResumeHandoffResult{
			Handoff: handoff,
			Prompt:  composedPrompt,
			Sent:    false,
		}, nil
	}

	return ResumeHandoffResult{
		Handoff: handoff,
		Prompt:  composedPrompt,
		Sent:    true,
	}, nil
}
