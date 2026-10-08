package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// TestPlanIncrementRoleStepHeuristicDoesNotOverScope locks in the scope
// boundary: a workflow that merely happens to contain one or two role-named
// steps is NOT the Role Pipeline and must keep the legacy no-criteria path.
// Only an explicit Role Pipeline workflow, an existing plan, or an approved
// receipt is scoped.
func TestPlanIncrementRoleStepHeuristicDoesNotOverScope(t *testing.T) {
	repo := newRepoForCompletionGateTests(t)
	ctx := context.Background()
	now := time.Now().UTC()

	_, err := repo.db.ExecContext(ctx, `
		INSERT INTO workflows (id, workspace_id, name, created_at, updated_at)
		VALUES ('wf-custom', 'ws-1', 'Custom Flow', ?, ?)
	`, now, now)
	if err != nil {
		t.Fatalf("insert workflow: %v", err)
	}
	// Two role-named steps: below the three-step structural signature.
	_, err = repo.db.ExecContext(ctx, `
		INSERT INTO workflow_steps (id, workflow_id, name, prompt, created_at, updated_at)
		VALUES ('step-arch', 'wf-custom', 'Architect', 'prompt', ?, ?),
		       ('step-impl', 'wf-custom', 'Implement', 'prompt', ?, ?)
	`, now, now, now, now)
	if err != nil {
		t.Fatalf("insert workflow steps: %v", err)
	}

	task := &models.Task{
		ID:             "task-custom-flow",
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf-custom",
		WorkflowStepID: "step-arch",
		Title:          "Custom Flow Task",
		State:          v1.TaskStateInProgress,
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	gate, err := repo.GetTaskCompletionGate(ctx, task.ID)
	if err != nil {
		t.Fatalf("get gate: %v", err)
	}
	if gate.Blocked || len(gate.Blockers) != 0 {
		t.Fatalf("expected non-Role-Pipeline task to stay legacy-unblocked, got %+v", gate)
	}

	// The Architect->Implement handoff and terminal move must both be allowed.
	tx, err := repo.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	handoffErr := repo.guardTaskCompletionTransitionTx(ctx, tx, task.ID, v1.TaskStateInProgress, v1.TaskStateInProgress, "wf-custom", "step-arch", "wf-custom", "step-impl")
	_ = tx.Rollback()
	if handoffErr != nil {
		t.Fatalf("legacy handoff must not be gated, got %v", handoffErr)
	}

	tx, err = repo.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	terminalErr := repo.guardTaskCompletionTransitionTx(ctx, tx, task.ID, v1.TaskStateInProgress, v1.TaskStateCompleted, "wf-custom", "step-impl", "wf-custom", "step-done")
	_ = tx.Rollback()
	if terminalErr != nil {
		t.Fatalf("legacy terminal move must not be gated, got %v", terminalErr)
	}
}
