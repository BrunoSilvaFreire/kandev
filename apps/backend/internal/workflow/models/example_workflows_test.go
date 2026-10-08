package models

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestRolePipelineExampleValidates loads the shipped Role Pipeline example and
// asserts it is a valid portable workflow with the expected named transitions
// and completion policy.
func TestRolePipelineExampleValidates(t *testing.T) {
	path := filepath.Join("..", "..", "..", "..", "..", "docs", "examples", "role-pipeline.workflow.yml")
	data, err := os.ReadFile(path)
	require.NoError(t, err, "role pipeline example must exist at %s", path)

	var export WorkflowExport
	require.NoError(t, yaml.Unmarshal(data, &export))
	require.NoError(t, export.Validate())
	require.Len(t, export.Workflows, 1)

	wf := export.Workflows[0]
	require.Len(t, wf.Steps, 6)

	transitionsByStep := map[string][]StepTransition{}
	for _, step := range wf.Steps {
		transitionsByStep[step.Name] = step.Events.Transitions
	}
	assert.Len(t, transitionsByStep["Architect"], 2)
	assert.Len(t, transitionsByStep["Implement"], 1)
	assert.Len(t, transitionsByStep["Review"], 2)
	assert.Equal(t, "handoff", transitionsByStep["Architect"][0].Name)
	assert.Equal(t, "request_exploration", transitionsByStep["Architect"][1].Name)
	assert.Equal(t, "escalate", transitionsByStep["Implement"][0].Name)
	assert.Equal(t, "request_changes", transitionsByStep["Review"][0].Name)
	assert.Equal(t, "pass", transitionsByStep["Review"][1].Name)

	// Assert completion policy: Done step completes on enter, earlier steps do not.
	for _, step := range wf.Steps {
		if step.Name == "Done" {
			assert.True(t, step.CompleteTaskOnEnter, "Done step must complete_task_on_enter")
		} else {
			assert.False(t, step.CompleteTaskOnEnter, "step %s must not complete_task_on_enter", step.Name)
		}
	}

	// The example must not carry the retired Plan Approval gate.
	for _, step := range wf.Steps {
		assert.NotEqual(t, "Plan Approval", step.Name)
	}

	// Prompts for Architect, Implement, and Review must reference the technical-design artifact.
	stepPromptByName := map[string]string{}
	for _, step := range wf.Steps {
		stepPromptByName[step.Name] = step.Prompt
	}
	assert.Contains(t, stepPromptByName["Architect"], "technical-design")
	assert.Contains(t, stepPromptByName["Implement"], "technical-design")
	assert.Contains(t, stepPromptByName["Review"], "technical-design")
}
