package models

import (
	"fmt"
	"regexp"
	"strings"
)

// Step transition directions. A forward transition must target a later step
// position, a backward transition an earlier one; the check runs at invocation
// time because positions are only known once both steps are loaded.
const (
	TransitionDirectionForward  = "forward"
	TransitionDirectionBackward = "backward"
)

// transitionNamePattern is the accepted shape of a transition name: a short,
// lowercase, URL-ish token that is stable enough to name in a prompt.
var transitionNamePattern = regexp.MustCompile(`^[a-z0-9_-]+$`)

// StepTransition is a named, agent-invoked alternative to the ordinary
// on_turn_complete transition. Transitions are stored under
// events.transitions in the existing workflow_steps.events JSON; the engine
// never evaluates them as triggers. An agent invokes one through
// move_task_kandev(transition=...).
type StepTransition struct {
	Name      string `json:"name" yaml:"name"`
	Direction string `json:"direction" yaml:"direction"`
	ToStepID  string `json:"to_step_id,omitempty" yaml:"to_step_id,omitempty"`
	// ToStepPosition is the portable form of the target: it is populated only
	// while an export/import round trips through positions, never in the
	// persisted domain model.
	ToStepPosition *int   `json:"to_step_position,omitempty" yaml:"to_step_position,omitempty"`
	Instructions   string `json:"instructions,omitempty" yaml:"instructions,omitempty"`
	SkipStepPrompt bool   `json:"skip_step_prompt,omitempty" yaml:"skip_step_prompt,omitempty"`
	ResetContext   bool   `json:"reset_context,omitempty" yaml:"reset_context,omitempty"`
}

// ValidateTransitions checks the shape of a step's named transitions: unique,
// well-formed names, a known direction, and a non-empty target.
func ValidateTransitions(transitions []StepTransition) error {
	seen := make(map[string]struct{}, len(transitions))
	for i, tr := range transitions {
		name := strings.TrimSpace(tr.Name)
		switch {
		case name == "":
			return fmt.Errorf("transitions[%d].name is required", i)
		case !transitionNamePattern.MatchString(name):
			return fmt.Errorf("transitions[%d].name %q must match [a-z0-9_-]+", i, name)
		case tr.Direction != TransitionDirectionForward && tr.Direction != TransitionDirectionBackward:
			return fmt.Errorf("transitions[%d].direction must be %q or %q", i, TransitionDirectionForward, TransitionDirectionBackward)
		case strings.TrimSpace(tr.ToStepID) == "" && tr.ToStepPosition == nil:
			return fmt.Errorf("transitions[%d] requires to_step_id or to_step_position", i)
		}
		if _, dup := seen[name]; dup {
			return fmt.Errorf("transitions[%d].name %q is duplicated", i, name)
		}
		seen[name] = struct{}{}
	}
	return nil
}

// FindTransition returns the named transition, or an error listing the
// available names so an agent can correct itself.
func FindTransition(events StepEvents, name string) (*StepTransition, error) {
	trimmed := strings.TrimSpace(name)
	available := make([]string, 0, len(events.Transitions))
	for i := range events.Transitions {
		tr := events.Transitions[i]
		if tr.Name == trimmed {
			return &tr, nil
		}
		available = append(available, tr.Name)
	}
	if len(available) == 0 {
		return nil, fmt.Errorf("step has no named transitions")
	}
	return nil, fmt.Errorf("unknown transition %q; available: %s", name, strings.Join(available, ", "))
}

// ValidateTransitionDirection enforces the direction invariant at invocation
// time: forward requires a larger target position, backward a smaller one.
func ValidateTransitionDirection(tr *StepTransition, sourcePosition, targetPosition int) error {
	if tr == nil {
		return fmt.Errorf("transition is required")
	}
	switch tr.Direction {
	case TransitionDirectionForward:
		if targetPosition <= sourcePosition {
			return fmt.Errorf("forward transition %q must target a later step", tr.Name)
		}
	case TransitionDirectionBackward:
		if targetPosition >= sourcePosition {
			return fmt.Errorf("backward transition %q must target an earlier step", tr.Name)
		}
	default:
		return fmt.Errorf("transition %q has unknown direction %q", tr.Name, tr.Direction)
	}
	return nil
}
