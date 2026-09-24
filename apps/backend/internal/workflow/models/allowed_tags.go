package models

import (
	"fmt"

	"github.com/kandev/kandev/internal/common/tags"
)

// ValidateAllowedTags canonicalizes a step's allowed tags and rejects the
// combination with a concrete session target or a configure_session action.
// Tag selection chooses the entry's initial profile, so it cannot coexist with
// a surface that already names one.
func ValidateAllowedTags(step *WorkflowStep) error {
	if step == nil {
		return nil
	}
	canonical, err := tags.Canonical(step.AllowedTags)
	if err != nil {
		return fmt.Errorf("invalid allowed_tags: %w", err)
	}
	step.AllowedTags = canonical
	if len(canonical) == 0 {
		return nil
	}
	if step.SessionTarget != nil {
		return fmt.Errorf("allowed_tags cannot be combined with session_target")
	}
	for _, action := range step.Events.OnEnter {
		if action.Type == OnEnterConfigureSession {
			return fmt.Errorf("allowed_tags cannot be combined with configure_session")
		}
	}
	return nil
}
