package models

import (
	"strings"
	"testing"
)

func TestValidateAllowedTagsCanonicalizes(t *testing.T) {
	step := &WorkflowStep{AllowedTags: []string{" Review ", "REVIEW", "sec"}}
	if err := ValidateAllowedTags(step); err != nil {
		t.Fatalf("validate allowed tags: %v", err)
	}
	if len(step.AllowedTags) != 2 || step.AllowedTags[0] != "review" || step.AllowedTags[1] != "sec" {
		t.Fatalf("allowed tags = %#v, want [review sec]", step.AllowedTags)
	}
}

func TestValidateAllowedTagsRejectsSessionTarget(t *testing.T) {
	step := &WorkflowStep{
		AllowedTags:   []string{"review"},
		SessionTarget: &WorkflowSessionTarget{Kind: WorkflowSessionTargetInitial},
	}
	if err := ValidateAllowedTags(step); err == nil {
		t.Fatal("expected rejection when allowed_tags combined with session_target")
	}
}

func TestValidateAllowedTagsRejectsConfigureSession(t *testing.T) {
	step := &WorkflowStep{
		AllowedTags: []string{"review"},
		Events: StepEvents{OnEnter: []OnEnterAction{
			{Type: OnEnterConfigureSession, Config: map[string]interface{}{"rules": []interface{}{
				map[string]interface{}{"agent_name": "claude-acp", "operation": "keep"},
			}}},
		}},
	}
	if err := ValidateWorkflowStep(step); err == nil || !strings.Contains(err.Error(), "allowed_tags cannot be combined with configure_session") {
		t.Fatalf("expected allowed_tags/configure_session rejection, got %v", err)
	}
}

func TestValidateAllowedTagsEmptyIsInert(t *testing.T) {
	step := &WorkflowStep{}
	if err := ValidateAllowedTags(step); err != nil {
		t.Fatalf("empty allowed tags should be inert: %v", err)
	}
	if step.AllowedTags == nil || len(step.AllowedTags) != 0 {
		t.Fatalf("allowed tags = %#v, want empty non-nil", step.AllowedTags)
	}
}
