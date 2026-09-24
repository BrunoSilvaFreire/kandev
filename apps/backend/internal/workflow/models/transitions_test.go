package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateTransitions(t *testing.T) {
	valid := []StepTransition{{Name: "handoff", Direction: TransitionDirectionForward, ToStepID: "step-3"}}
	require.NoError(t, ValidateTransitions(valid))
	require.NoError(t, ValidateTransitions(nil))

	cases := map[string][]StepTransition{
		"empty name":    {{Name: "", Direction: TransitionDirectionForward, ToStepID: "s"}},
		"bad name":      {{Name: "Hand Off!", Direction: TransitionDirectionForward, ToStepID: "s"}},
		"bad direction": {{Name: "handoff", Direction: "sideways", ToStepID: "s"}},
		"missing target": {
			{Name: "handoff", Direction: TransitionDirectionForward},
		},
		"duplicate": {
			{Name: "handoff", Direction: TransitionDirectionForward, ToStepID: "s"},
			{Name: "handoff", Direction: TransitionDirectionBackward, ToStepID: "s2"},
		},
	}
	for name, transitions := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Error(t, ValidateTransitions(transitions))
		})
	}

	// Portable form (to_step_position only) is accepted.
	pos := 3
	require.NoError(t, ValidateTransitions([]StepTransition{{
		Name: "handoff", Direction: TransitionDirectionForward, ToStepPosition: &pos,
	}}))
}

func TestFindTransitionListsAvailable(t *testing.T) {
	events := StepEvents{Transitions: []StepTransition{
		{Name: "handoff", Direction: TransitionDirectionForward, ToStepID: "s3"},
		{Name: "escalate", Direction: TransitionDirectionBackward, ToStepID: "s2"},
	}}
	tr, err := FindTransition(events, "escalate")
	require.NoError(t, err)
	assert.Equal(t, "s2", tr.ToStepID)

	_, err = FindTransition(events, "nope")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "handoff")
	assert.Contains(t, err.Error(), "escalate")

	_, err = FindTransition(StepEvents{}, "anything")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no named transitions")
}

func TestValidateTransitionDirection(t *testing.T) {
	fwd := &StepTransition{Name: "handoff", Direction: TransitionDirectionForward}
	require.NoError(t, ValidateTransitionDirection(fwd, 2, 3))
	require.Error(t, ValidateTransitionDirection(fwd, 3, 2))
	require.Error(t, ValidateTransitionDirection(fwd, 2, 2))

	bwd := &StepTransition{Name: "escalate", Direction: TransitionDirectionBackward}
	require.NoError(t, ValidateTransitionDirection(bwd, 3, 2))
	require.Error(t, ValidateTransitionDirection(bwd, 2, 3))
}

func TestCollectAndRemapTransitions(t *testing.T) {
	events := StepEvents{Transitions: []StepTransition{
		{Name: "handoff", Direction: TransitionDirectionForward, ToStepID: "old-3"},
	}}
	refs := CollectStepEventReferences(events)
	assert.Equal(t, []string{"old-3"}, refs.StepIDs)

	remapped := RemapStepEvents(events, map[string]string{"old-3": "new-3"})
	require.Len(t, remapped.Transitions, 1)
	assert.Equal(t, "new-3", remapped.Transitions[0].ToStepID)
	assert.Equal(t, "handoff", remapped.Transitions[0].Name)
}

func TestTransitionPositionRoundTrip(t *testing.T) {
	events := StepEvents{Transitions: []StepTransition{
		{Name: "handoff", Direction: TransitionDirectionForward, ToStepID: "step-c", Instructions: "go"},
	}}
	portable := convertStepIDToPosition(events, map[string]int{"step-c": 3})
	require.Len(t, portable.Transitions, 1)
	require.NotNil(t, portable.Transitions[0].ToStepPosition)
	assert.Equal(t, 3, *portable.Transitions[0].ToStepPosition)
	assert.Empty(t, portable.Transitions[0].ToStepID)
	assert.Equal(t, "go", portable.Transitions[0].Instructions)

	back := ConvertPositionToStepID(portable, map[int]string{3: "step-c"})
	require.Len(t, back.Transitions, 1)
	assert.Equal(t, "step-c", back.Transitions[0].ToStepID)
	assert.Nil(t, back.Transitions[0].ToStepPosition)
}

func TestValidateStepPositionRefsRejectsBadTransitionPosition(t *testing.T) {
	pos := 9
	steps := []StepPortable{{
		Name: "Architect",
		Events: StepEvents{Transitions: []StepTransition{
			{Name: "handoff", Direction: TransitionDirectionForward, ToStepPosition: &pos},
		}},
	}}
	err := validateStepPositionRefs(steps, map[int]bool{0: true, 1: true, 2: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "to_step_position 9 does not match any step")

	missing := []StepPortable{{
		Name: "Architect",
		Events: StepEvents{Transitions: []StepTransition{
			{Name: "handoff", Direction: TransitionDirectionForward},
		}},
	}}
	err = validateStepPositionRefs(missing, map[int]bool{0: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "to_step_position is required")
}
