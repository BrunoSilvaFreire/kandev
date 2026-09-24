package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestApprovalToolRegisteredInTaskMode(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	s := New(backend, "test-session", "test-task", 10005, log, "", false, ModeTask)
	assert.Contains(t, getRegisteredToolNames(s), "request_approval_kandev")
}

func TestValidateApprovalRequestArgs(t *testing.T) {
	require.Nil(t, validateApprovalRequestArgs("task_plan", "", "Plan", "summary"))
	require.NotNil(t, validateApprovalRequestArgs("document", "", "Plan", "summary"))
	require.Nil(t, validateApprovalRequestArgs("document", "review", "Plan", "summary"))
	require.NotNil(t, validateApprovalRequestArgs("nope", "", "Plan", "summary"))
	require.NotNil(t, validateApprovalRequestArgs("task_plan", "", "", "summary"))
	require.NotNil(t, validateApprovalRequestArgs("task_plan", "", "Plan", ""))
	require.NotNil(t, validateApprovalRequestArgs("task_plan", "", string(make([]rune, 61)), "summary"))
}

func TestExtractApprovalOutcomeShape(t *testing.T) {
	result := map[string]interface{}{
		"approval": map[string]interface{}{
			"decision":        "revise",
			"feedback":        "tighten scope",
			"plan_comments":   "### Plan Comments\n\n> fix\n",
			"comment_ids":     []interface{}{"c1"},
			"subject_edited":  true,
			"current_version": "v3",
		},
	}
	toolResult := extractApprovalOutcome(result)
	require.NotNil(t, toolResult)
	require.NotNil(t, toolResult.StructuredContent)
	out, ok := toolResult.StructuredContent.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "revise", out["decision"])
	assert.Equal(t, "tighten scope", out["feedback"])
	assert.Equal(t, true, out["subject_edited"])
	assert.Equal(t, "v3", out["current_version"])
}
