package repository

import (
	"context"
	"errors"
	"testing"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/workflow/models"
)

func TestApplyWorkflowDefinitionChecksVersionAndCommitsAtomically(t *testing.T) {
	repo, database := setupTestRepoWithDB(t)
	ctx := context.Background()
	step := &models.WorkflowStep{ID: "definition-step", WorkflowID: "wf-test", Name: "Before", Position: 0}
	if err := repo.CreateStep(ctx, step); err != nil {
		t.Fatal(err)
	}
	version, err := repo.WorkflowDefinitionVersion(ctx, "wf-test")
	if err != nil {
		t.Fatal(err)
	}
	workflow := &taskmodels.Workflow{ID: "wf-test", Name: "Changed", Description: "description", Prompt: "instructions"}
	candidate := *step
	candidate.Name = "After"
	if _, err := repo.ApplyWorkflowDefinition(ctx, "wf-test", "stale-token", workflow, []*models.WorkflowStep{&candidate}, nil); !errors.Is(err, repoerrors.ErrTaskVersionConflict) {
		t.Fatalf("stale update error = %v, want version conflict", err)
	}
	var unchangedName string
	if err := database.Get(&unchangedName, `SELECT name FROM workflows WHERE id = ?`, "wf-test"); err != nil || unchangedName != "Test" {
		t.Fatalf("workflow after stale update = %q, error = %v", unchangedName, err)
	}
	changed, err := repo.ApplyWorkflowDefinition(ctx, "wf-test", version, workflow, []*models.WorkflowStep{&candidate}, nil)
	if err != nil || !changed {
		t.Fatalf("apply changed=%t error=%v", changed, err)
	}
	updatedWorkflow, err := repo.WorkflowDefinitionVersion(ctx, "wf-test")
	if err != nil || updatedWorkflow == version {
		t.Fatalf("updated version = %q, previous = %q, error = %v", updatedWorkflow, version, err)
	}
	updatedStep, err := repo.GetStep(ctx, step.ID)
	if err != nil || updatedStep.Name != "After" {
		t.Fatalf("updated step = %#v, error = %v", updatedStep, err)
	}
}

func TestApplyWorkflowDefinitionDoesNotWriteNoDrift(t *testing.T) {
	repo, _ := setupTestRepoWithDB(t)
	ctx := context.Background()
	step := &models.WorkflowStep{ID: "same-step", WorkflowID: "wf-test", Name: "Same", Position: 0}
	if err := repo.CreateStep(ctx, step); err != nil {
		t.Fatal(err)
	}
	version, err := repo.WorkflowDefinitionVersion(ctx, "wf-test")
	if err != nil {
		t.Fatal(err)
	}
	steps, err := repo.ListStepsByWorkflow(ctx, "wf-test")
	if err != nil {
		t.Fatal(err)
	}
	workflow := &taskmodels.Workflow{ID: "wf-test", Name: "Test"}
	changed, err := repo.ApplyWorkflowDefinition(ctx, "wf-test", version, workflow, steps, nil)
	if err != nil || changed {
		t.Fatalf("no-drift apply changed=%t error=%v", changed, err)
	}
}
