package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// TestPlanIncrementEnrolledUnverifiedHandoff is a regression test: an enrolled
// plan increment set is necessarily unverified when the Architect hands the task
// to Implement (the Review step verifies the increments after implementation).
// The Architect->Implement handoff must therefore succeed even though
// snapshot.Blocked is true; only an unenrolled or stale plan fails closed.
func TestPlanIncrementEnrolledUnverifiedHandoff(t *testing.T) {
	repo := newRepoForCompletionGateTests(t)
	ctx := context.Background()
	now := time.Now().UTC()

	_, err := repo.db.ExecContext(ctx, `
		INSERT INTO workflows (id, workspace_id, name, created_at, updated_at)
		VALUES ('wf-role-enrolled', 'ws-1', 'Role Pipeline', ?, ?)
	`, now, now)
	if err != nil {
		t.Fatalf("insert workflow: %v", err)
	}
	_, err = repo.db.ExecContext(ctx, `
		INSERT INTO workflow_steps (id, workflow_id, name, prompt, created_at, updated_at)
		VALUES ('step-arch-enrolled', 'wf-role-enrolled', 'Architect', 'prompt', ?, ?),
		       ('step-impl-enrolled', 'wf-role-enrolled', 'Implement', 'prompt', ?, ?),
		       ('step-done-enrolled', 'wf-role-enrolled', 'Done', 'prompt', ?, ?)
	`, now, now, now, now, now, now)
	if err != nil {
		t.Fatalf("insert workflow steps: %v", err)
	}

	task := &models.Task{
		ID:             "task-enrolled-handoff",
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf-role-enrolled",
		WorkflowStepID: "step-arch-enrolled",
		Title:          "Enrolled Handoff Task",
		State:          v1.TaskStateInProgress,
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	seedApprovedPlanRevision(t, ctx, repo, task.ID, "rev-enrolled-1")
	enrolled, err := repo.SetTaskCompletionCriteria(ctx, models.TaskCompletionCriteriaChange{
		TaskID:           task.ID,
		WorkspaceID:      task.WorkspaceID,
		ExpectedRevision: 0,
		PlanRevisionID:   "rev-enrolled-1",
		ActorKind:        "agent",
		ActorID:          "architect-agent",
		Criteria: []models.TaskCompletionCriterion{
			{ID: "I1", Description: "Increment 1", EvidenceSubject: models.TaskCompletionEvidenceSubject{Kind: models.TaskCompletionEvidencePlanIncrement, ID: "I1"}},
		},
	})
	if err != nil {
		t.Fatalf("enroll plan increments: %v", err)
	}
	if !enrolled.Blocked {
		t.Fatal("enrolled but unverified criteria must report Blocked = true")
	}
	if enrolled.PlanStale {
		t.Fatal("enrolled plan must not be stale immediately after enrollment")
	}

	// Architect -> Implement handoff must SUCCEED while criteria are unverified.
	tx, err := repo.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	handoffErr := repo.guardTaskCompletionTransitionTx(ctx, tx, task.ID, v1.TaskStateInProgress, v1.TaskStateInProgress, "wf-role-enrolled", "step-arch-enrolled", "wf-role-enrolled", "step-impl-enrolled")
	_ = tx.Rollback()
	if handoffErr != nil {
		t.Fatalf("architect-to-implement handoff with enrolled unverified criteria = %v, want nil", handoffErr)
	}

	// Terminal completion must still fail closed while criteria are unverified.
	tx, err = repo.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	terminalErr := repo.guardTaskCompletionTransitionTx(ctx, tx, task.ID, v1.TaskStateInProgress, v1.TaskStateCompleted, "wf-role-enrolled", "step-impl-enrolled", "wf-role-enrolled", "step-done-enrolled")
	_ = tx.Rollback()
	if !errors.Is(terminalErr, repoerrors.ErrTaskCompletionGateBlocked) {
		t.Fatalf("terminal transition with unverified criteria = %v, want ErrTaskCompletionGateBlocked", terminalErr)
	}
}
