package engine

import (
	"testing"

	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCompileStepIgnoresNamedTransitions pins that named transitions are an
// invocation-time lookup, never an engine trigger: compiling a step with
// transitions must produce exactly the same StepSpec as compiling it without.
func TestCompileStepIgnoresNamedTransitions(t *testing.T) {
	base := &wfmodels.WorkflowStep{
		ID: "step-2", WorkflowID: "wf", Name: "Architect", Position: 2,
	}
	withTransitions := *base
	withTransitions.Events = wfmodels.StepEvents{Transitions: []wfmodels.StepTransition{
		{Name: "handoff", Direction: wfmodels.TransitionDirectionForward, ToStepID: "step-3"},
	}}

	assert.Equal(t, CompileStep(base), CompileStep(&withTransitions))
	require.Empty(t, CompileStep(&withTransitions).Events[TriggerOnTurnComplete])
}
