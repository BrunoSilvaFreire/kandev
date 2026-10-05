package clarification

import (
	"context"
	"strings"
	"time"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

// continuationRecoveryHandlerTimeout bounds the orchestrator's retry/continue
// work (extraction, reset, send) when it runs detached from the request that
// answered the bundle.
const continuationRecoveryHandlerTimeout = 5 * time.Minute

// ContinuationRecoveryHandler applies a continuation-recovery decision to the
// paused automatic continuation it names. It is implemented by the
// orchestrator and injected into the Resolver; the resolver authorizes through
// the bundle's task before calling it, and a decision whose stamp no longer
// matches the pending record must be a no-op.
type ContinuationRecoveryHandler interface {
	ResolveContinuationRecovery(ctx context.Context, taskID, stamp, decision string) error
}

// ContinuationRecoveryQuestion returns the fixed single question of a
// continuation-recovery bundle. The option IDs are the stable decisions the
// resolver forwards; the labels are English placeholders the web card
// replaces with localized copy, mirroring the approval card.
func ContinuationRecoveryQuestion() Question {
	return Question{
		ID:     ContinuationRecoveryQuestionID,
		Title:  "Continue?",
		Prompt: "Automatic continuation could not prepare a handoff. Retry, or continue without one?",
		Options: []Option{
			{ID: ContinuationRecoveryDecisionRetry, Label: "Try again", Description: "Retry preparing the handoff, then send the step prompt."},
			{ID: ContinuationRecoveryDecisionContinue, Label: "Continue without handoff", Description: "Send the step prompt now, without a handoff."},
		},
	}
}

// IsContinuationRecoveryBundle reports whether msgs belong to a
// continuation-recovery request.
func IsContinuationRecoveryBundle(msgs []*taskmodels.Message) bool {
	return continuationRecoveryMetaFromMessages(msgs) != nil
}

// continuationRecoveryMetaFromMessages extracts the recovery metadata persisted
// on the bundle's messages. It returns nil for any other bundle.
func continuationRecoveryMetaFromMessages(msgs []*taskmodels.Message) *ContinuationRecoveryMeta {
	for _, m := range msgs {
		if m == nil || m.Metadata == nil {
			continue
		}
		raw, ok := m.Metadata["continuation_recovery"]
		if !ok {
			continue
		}
		if meta := decodeContinuationRecoveryMeta(raw); meta != nil {
			return meta
		}
	}
	return nil
}

func decodeContinuationRecoveryMeta(raw any) *ContinuationRecoveryMeta {
	switch v := raw.(type) {
	case *ContinuationRecoveryMeta:
		if v == nil || strings.TrimSpace(v.Stamp) == "" {
			return nil
		}
		return v
	case ContinuationRecoveryMeta:
		if strings.TrimSpace(v.Stamp) == "" {
			return nil
		}
		return &v
	case map[string]any:
		meta := &ContinuationRecoveryMeta{}
		meta.Stamp, _ = v["stamp"].(string)
		meta.Case, _ = v["case"].(string)
		meta.Reason, _ = v["reason"].(string)
		if strings.TrimSpace(meta.Stamp) == "" {
			return nil
		}
		return meta
	default:
		return nil
	}
}

// validateContinuationRecoveryBundle enforces the fixed shape of a
// continuation-recovery request: exactly one question whose option IDs are the
// two stable decisions.
func validateContinuationRecoveryBundle(questions []Question, meta *ContinuationRecoveryMeta) error {
	if meta == nil {
		return nil
	}
	if strings.TrimSpace(meta.Stamp) == "" {
		return newValidationError("continuation_recovery bundle requires a stamp")
	}
	if len(questions) != 1 {
		return newValidationError("continuation_recovery bundle must have exactly one question")
	}
	q := questions[0]
	if q.ID != ContinuationRecoveryQuestionID {
		return newValidationError("continuation_recovery bundle question id must be %q", ContinuationRecoveryQuestionID)
	}
	valid := map[string]bool{
		ContinuationRecoveryDecisionRetry:    false,
		ContinuationRecoveryDecisionContinue: false,
	}
	for _, opt := range q.Options {
		if _, ok := valid[opt.ID]; ok {
			valid[opt.ID] = true
		}
	}
	for id, seen := range valid {
		if !seen {
			return newValidationError("continuation_recovery bundle is missing option %q", id)
		}
	}
	return nil
}

// continuationRecoveryDecision extracts the selected decision from a validated
// outcome.
func continuationRecoveryDecision(outcome Outcome) (string, error) {
	if outcome.Rejected || len(outcome.Answers) != 1 {
		return "", newValidationError("continuation_recovery bundle expects exactly one answer")
	}
	selected := outcome.Answers[0].SelectedOptions
	if len(selected) != 1 {
		return "", newValidationError("continuation_recovery answer must select exactly one decision")
	}
	switch selected[0] {
	case ContinuationRecoveryDecisionRetry, ContinuationRecoveryDecisionContinue:
		return selected[0], nil
	default:
		return "", newValidationError("continuation_recovery answer references unknown decision %q", selected[0])
	}
}
