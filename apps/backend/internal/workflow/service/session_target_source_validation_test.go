package service

import (
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/workflow/models"
)

func TestValidateWorkflowSessionTargetsRejectsTaggedSource(t *testing.T) {
	dest := &models.WorkflowStep{
		ID: "review", WorkflowID: "wf", Position: 2,
		SessionTarget: &models.WorkflowSessionTarget{
			Kind: models.WorkflowSessionTargetStep, StepID: "implement",
		},
	}
	source := &models.WorkflowStep{
		ID: "implement", WorkflowID: "wf", Position: 1,
		AgentProfileID: "profile-a", AllowedTags: []string{"review"},
	}
	err := validateWorkflowSessionTargets([]*models.WorkflowStep{source, dest})
	if err == nil || !strings.Contains(err.Error(), "direct agent profile") {
		t.Fatalf("err = %v, want a direct-agent-profile rejection for a tagged source", err)
	}
}

func TestValidateWorkflowSessionTargetsAcceptsDirectSource(t *testing.T) {
	dest := &models.WorkflowStep{
		ID: "review", WorkflowID: "wf", Position: 2,
		SessionTarget: &models.WorkflowSessionTarget{
			Kind: models.WorkflowSessionTargetStep, StepID: "implement",
		},
	}
	source := &models.WorkflowStep{
		ID: "implement", WorkflowID: "wf", Position: 1, AgentProfileID: "profile-a",
	}
	if err := validateWorkflowSessionTargets([]*models.WorkflowStep{source, dest}); err != nil {
		t.Fatalf("validateWorkflowSessionTargets: %v", err)
	}
}
