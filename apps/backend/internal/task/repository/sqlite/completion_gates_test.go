package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func newRepoForCompletionGateTests(t *testing.T) *Repository {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "completion-gates.db")
	dbConn, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	repo, err := NewWithDB(sqlxDB, sqlxDB, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	t.Cleanup(func() { _ = sqlxDB.Close() })
	return repo
}

func createTestTaskForGate(t *testing.T, ctx context.Context, repo *Repository, taskID string) *models.Task {
	t.Helper()
	task := &models.Task{
		ID:          taskID,
		WorkspaceID: "ws-1",
		Title:       "Test Gated Task",
		State:       v1.TaskStateInProgress,
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	return task
}

func seedApprovedPlanRevision(t *testing.T, ctx context.Context, repo *Repository, taskID, revID string) {
	t.Helper()
	now := time.Now().UTC()
	writeVersion := "wv-" + revID
	_, err := repo.db.ExecContext(ctx, `
		INSERT INTO task_plans (id, task_id, title, content, created_by, created_at, updated_at, write_version)
		VALUES (?, ?, 'Plan', 'Plan content', 'agent', ?, ?, ?)
		ON CONFLICT(task_id) DO UPDATE SET write_version = excluded.write_version
	`, "plan-"+taskID, taskID, now, now, writeVersion)
	if err != nil {
		t.Fatalf("insert task plan: %v", err)
	}
	var maxRev int
	_ = repo.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(revision_number), 0) FROM task_plan_revisions WHERE task_id = ?`, taskID).Scan(&maxRev)
	nextRev := maxRev + 1
	_, err = repo.db.ExecContext(ctx, `
		INSERT INTO task_plan_revisions (id, task_id, revision_number, title, content, author_kind, author_name, workflow_step_id, created_at, updated_at, write_version)
		VALUES (?, ?, ?, 'Plan', 'Plan content', 'agent', 'architect', 'step-plan', ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET write_version = excluded.write_version
	`, revID, taskID, nextRev, now, now, writeVersion)
	if err != nil {
		t.Fatalf("insert plan revision: %v", err)
	}
	_, err = repo.db.ExecContext(ctx, `
		INSERT INTO task_plan_approval_receipts (id, task_id, plan_revision_id, write_version, decision, subject_edited, created_at)
		VALUES (?, ?, ?, ?, 'approve', 0, ?)
		ON CONFLICT(id) DO NOTHING
	`, "receipt-"+revID, taskID, revID, writeVersion, now)
	if err != nil {
		t.Fatalf("insert approval receipt: %v", err)
	}
}

func TestPlanIncrementCompletionCriterionLifecycle(t *testing.T) {
	repo := newRepoForCompletionGateTests(t)
	ctx := context.Background()
	task := createTestTaskForGate(t, ctx, repo, "task-increment-lifecycle")
	seedApprovedPlanRevision(t, ctx, repo, task.ID, "rev-approved-1")

	// 1. Set criteria declaring increments I1 and I2 with PlanRevisionID
	criteriaChange := models.TaskCompletionCriteriaChange{
		TaskID:           task.ID,
		WorkspaceID:      task.WorkspaceID,
		ExpectedRevision: 0,
		PlanRevisionID:   "rev-approved-1",
		ActorKind:        "agent",
		ActorID:          "architect-agent",
		Criteria: []models.TaskCompletionCriterion{
			{
				ID:          "I1",
				Description: "Increment 1: Core contracts",
				EvidenceSubject: models.TaskCompletionEvidenceSubject{
					Kind: models.TaskCompletionEvidencePlanIncrement,
					ID:   "I1",
				},
			},
			{
				ID:          "I2",
				Description: "Increment 2: Implementation",
				EvidenceSubject: models.TaskCompletionEvidenceSubject{
					Kind: models.TaskCompletionEvidencePlanIncrement,
					ID:   "I2",
				},
			},
		},
	}

	snapshot, err := repo.SetTaskCompletionCriteria(ctx, criteriaChange)
	if err != nil {
		t.Fatalf("set completion criteria: %v", err)
	}
	if snapshot.Revision != 1 {
		t.Fatalf("snapshot revision = %d, want 1", snapshot.Revision)
	}
	if snapshot.PlanRevisionID != "rev-approved-1" {
		t.Fatalf("snapshot plan revision ID = %q, want rev-approved-1", snapshot.PlanRevisionID)
	}
	if !snapshot.Blocked {
		t.Fatal("expected snapshot to be blocked")
	}
	if len(snapshot.Blockers) != 2 {
		t.Fatalf("blockers count = %d, want 2", len(snapshot.Blockers))
	}
	for _, b := range snapshot.Blockers {
		if b.Reason != "unverified" {
			t.Errorf("blocker for %s reason = %q, want unverified", b.CriterionID, b.Reason)
		}
	}

	// 2. Terminal transition must be blocked
	tx, err := repo.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	guardErr := repo.guardTaskCompletionTransitionTx(ctx, tx, task.ID, v1.TaskStateInProgress, v1.TaskStateCompleted, "wf-1", "step-review", "wf-1", "step-done")
	_ = tx.Rollback()
	if !errors.Is(guardErr, repoerrors.ErrTaskCompletionGateBlocked) {
		t.Fatalf("terminal transition guard error = %v, want ErrTaskCompletionGateBlocked", guardErr)
	}

	// 3. Verify I1 with valid plan_increment evidence (revision "1")
	verifyI1 := models.TaskCompletionEvidenceChange{
		TaskID:           task.ID,
		WorkspaceID:      task.WorkspaceID,
		ExpectedRevision: snapshot.Revision,
		CriterionID:      "I1",
		ActorKind:        "agent",
		ActorID:          "reviewer-agent",
		Evidence: models.TaskCompletionEvidence{
			Subject: models.TaskCompletionEvidenceSubject{
				Kind:     models.TaskCompletionEvidencePlanIncrement,
				ID:       "I1",
				Revision: "1",
			},
			Summary:   "Increment 1 reviewed and confirmed",
			Reference: "review-doc-1",
		},
	}
	snapshot, err = repo.VerifyTaskCompletionCriterion(ctx, verifyI1)
	if err != nil {
		t.Fatalf("verify I1: %v", err)
	}
	// Still blocked because I2 is unverified
	if !snapshot.Blocked || len(snapshot.Blockers) != 1 || snapshot.Blockers[0].CriterionID != "I2" {
		t.Fatalf("snapshot after verifying I1 = %+v, want only I2 blocking", snapshot)
	}

	// 4. Test Independence: Mutate task.updated_at and task fields
	time.Sleep(10 * time.Millisecond)
	task.Title = "Updated Task Title (Next Turn)"
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("update task: %v", err)
	}
	reloaded, err := repo.GetTaskCompletionGate(ctx, task.ID)
	if err != nil {
		t.Fatalf("get gate after task update: %v", err)
	}
	// I1 MUST STILL BE VERIFIED!
	var i1Criterion *models.TaskCompletionCriterion
	for _, c := range reloaded.Criteria {
		if c.ID == "I1" {
			i1Criterion = &c
			break
		}
	}
	if i1Criterion == nil || i1Criterion.VerifiedRevision != 1 {
		t.Fatalf("I1 criterion after task update = %+v, want verified revision 1", i1Criterion)
	}

	// 5. Verify I2 with valid plan_increment evidence (revision "1")
	verifyI2 := models.TaskCompletionEvidenceChange{
		TaskID:           task.ID,
		WorkspaceID:      task.WorkspaceID,
		ExpectedRevision: snapshot.Revision,
		CriterionID:      "I2",
		ActorKind:        "agent",
		ActorID:          "reviewer-agent",
		Evidence: models.TaskCompletionEvidence{
			Subject: models.TaskCompletionEvidenceSubject{
				Kind:     models.TaskCompletionEvidencePlanIncrement,
				ID:       "I2",
				Revision: "1",
			},
			Summary: "Increment 2 reviewed and confirmed",
		},
	}
	snapshot, err = repo.VerifyTaskCompletionCriterion(ctx, verifyI2)
	if err != nil {
		t.Fatalf("verify I2: %v", err)
	}
	if snapshot.Blocked || len(snapshot.Blockers) != 0 {
		t.Fatalf("snapshot after verifying I2 = %+v, want completely unblocked", snapshot)
	}

	// 6. Terminal transition must now SUCCEED
	tx, err = repo.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	guardErr = repo.guardTaskCompletionTransitionTx(ctx, tx, task.ID, v1.TaskStateInProgress, v1.TaskStateCompleted, "wf-1", "step-review", "wf-1", "step-done")
	_ = tx.Rollback()
	if guardErr != nil {
		t.Fatalf("terminal transition guard error after full verification = %v, want nil", guardErr)
	}

	// 7. Modifying an increment's definition bumps criterion revision and invalidates prior proof
	seedApprovedPlanRevision(t, ctx, repo, task.ID, "rev-approved-2")
	modifyI1Change := models.TaskCompletionCriteriaChange{
		TaskID:           task.ID,
		WorkspaceID:      task.WorkspaceID,
		ExpectedRevision: snapshot.Revision,
		PlanRevisionID:   "rev-approved-2",
		ActorKind:        "agent",
		ActorID:          "architect-agent",
		Criteria: []models.TaskCompletionCriterion{
			{
				ID:          "I1",
				Description: "Increment 1: Core contracts (EXPANDED SCOPE)",
				EvidenceSubject: models.TaskCompletionEvidenceSubject{
					Kind: models.TaskCompletionEvidencePlanIncrement,
					ID:   "I1",
				},
			},
			{
				ID:          "I2",
				Description: "Increment 2: Implementation",
				EvidenceSubject: models.TaskCompletionEvidenceSubject{
					Kind: models.TaskCompletionEvidencePlanIncrement,
					ID:   "I2",
				},
			},
		},
	}
	snapshot, err = repo.SetTaskCompletionCriteria(ctx, modifyI1Change)
	if err != nil {
		t.Fatalf("modify criterion I1: %v", err)
	}
	if !snapshot.Blocked {
		t.Fatal("expected snapshot to be blocked after modifying I1")
	}
	// I1 was modified so its criterion_revision became 2 and its proof is unverified/stale.
	// I2 was unchanged so its verified proof remains!
	if len(snapshot.Blockers) != 1 || snapshot.Blockers[0].CriterionID != "I1" {
		t.Fatalf("blockers after modifying I1 = %+v, want only I1 blocked", snapshot.Blockers)
	}

	// 8. Re-verifying I1 with new revision "2" unblocks it
	reverifyI1 := models.TaskCompletionEvidenceChange{
		TaskID:           task.ID,
		WorkspaceID:      task.WorkspaceID,
		ExpectedRevision: snapshot.Revision,
		CriterionID:      "I1",
		ActorKind:        "agent",
		ActorID:          "reviewer-agent",
		Evidence: models.TaskCompletionEvidence{
			Subject: models.TaskCompletionEvidenceSubject{
				Kind:     models.TaskCompletionEvidencePlanIncrement,
				ID:       "I1",
				Revision: "2",
			},
			Summary: "Increment 1 reverified after scope adjustment",
		},
	}
	snapshot, err = repo.VerifyTaskCompletionCriterion(ctx, reverifyI1)
	if err != nil {
		t.Fatalf("reverify I1: %v", err)
	}
	if snapshot.Blocked {
		t.Fatalf("snapshot after reverifying I1 with revision 2 = %+v, want unblocked", snapshot)
	}
}

func TestPlanIncrementHumanConfirmationForWeakening(t *testing.T) {
	repo := newRepoForCompletionGateTests(t)
	ctx := context.Background()
	task := createTestTaskForGate(t, ctx, repo, "task-increment-weakening")

	// Set criteria I1 and I2
	snapshot, err := repo.SetTaskCompletionCriteria(ctx, models.TaskCompletionCriteriaChange{
		TaskID:           task.ID,
		WorkspaceID:      task.WorkspaceID,
		ExpectedRevision: 0,
		ActorKind:        "human",
		ActorID:          "user-1",
		Criteria: []models.TaskCompletionCriterion{
			{ID: "I1", Description: "Increment 1", EvidenceSubject: models.TaskCompletionEvidenceSubject{Kind: models.TaskCompletionEvidencePlanIncrement, ID: "I1"}},
			{ID: "I2", Description: "Increment 2", EvidenceSubject: models.TaskCompletionEvidenceSubject{Kind: models.TaskCompletionEvidencePlanIncrement, ID: "I2"}},
		},
	})
	if err != nil {
		t.Fatalf("initial criteria set: %v", err)
	}

	// Attempting to remove unmet I2 without human confirmation fails
	_, err = repo.SetTaskCompletionCriteria(ctx, models.TaskCompletionCriteriaChange{
		TaskID:           task.ID,
		WorkspaceID:      task.WorkspaceID,
		ExpectedRevision: snapshot.Revision,
		ActorKind:        "agent",
		ActorID:          "agent-1",
		Criteria: []models.TaskCompletionCriterion{
			{ID: "I1", Description: "Increment 1", EvidenceSubject: models.TaskCompletionEvidenceSubject{Kind: models.TaskCompletionEvidencePlanIncrement, ID: "I1"}},
		},
	})
	if !errors.Is(err, repoerrors.ErrTaskCompletionHumanConfirmationRequired) {
		t.Fatalf("agent removal of unmet criterion error = %v, want ErrTaskCompletionHumanConfirmationRequired", err)
	}

	// Removing with human confirmation succeeds
	removed, err := repo.SetTaskCompletionCriteria(ctx, models.TaskCompletionCriteriaChange{
		TaskID:                    task.ID,
		WorkspaceID:               task.WorkspaceID,
		ExpectedRevision:          snapshot.Revision,
		ActorKind:                 "human",
		ActorID:                   "user-1",
		HumanConfirmationRevision: snapshot.Revision,
		HumanConfirmationReason:   "Scope reduced for MVP",
		Criteria: []models.TaskCompletionCriterion{
			{ID: "I1", Description: "Increment 1", EvidenceSubject: models.TaskCompletionEvidenceSubject{Kind: models.TaskCompletionEvidencePlanIncrement, ID: "I1"}},
		},
	})
	if err != nil {
		t.Fatalf("human confirmed removal: %v", err)
	}
	if len(removed.Criteria) != 1 || removed.Criteria[0].ID != "I1" {
		t.Fatalf("criteria after removal = %+v", removed.Criteria)
	}
}

func TestPlanIncrementHumanOverrideAudit(t *testing.T) {
	repo := newRepoForCompletionGateTests(t)
	ctx := context.Background()
	task := createTestTaskForGate(t, ctx, repo, "task-increment-override")

	snapshot, err := repo.SetTaskCompletionCriteria(ctx, models.TaskCompletionCriteriaChange{
		TaskID:           task.ID,
		WorkspaceID:      task.WorkspaceID,
		ExpectedRevision: 0,
		ActorKind:        "human",
		ActorID:          "user-1",
		Criteria: []models.TaskCompletionCriterion{
			{ID: "I1", Description: "Increment 1", EvidenceSubject: models.TaskCompletionEvidenceSubject{Kind: models.TaskCompletionEvidencePlanIncrement, ID: "I1"}},
		},
	})
	if err != nil {
		t.Fatalf("set criteria: %v", err)
	}

	overrideCtx := models.WithTaskCompletionMoveOverride(ctx, models.TaskCompletionMoveOverride{
		TaskID:           task.ID,
		WorkspaceID:      task.WorkspaceID,
		ExpectedRevision: snapshot.Revision,
		SourceWorkflowID: "wf-1",
		SourceStepID:     "step-review",
		TargetWorkflowID: "wf-1",
		TargetStepID:     "step-done",
		ActorID:          "operator-1",
		Reason:           "Urgent hotfix release approved out of band",
	})

	tx, err := repo.db.BeginTx(overrideCtx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	guardErr := repo.guardTaskCompletionTransitionTx(overrideCtx, tx, task.ID, v1.TaskStateInProgress, v1.TaskStateCompleted, "wf-1", "step-review", "wf-1", "step-done")
	if guardErr != nil {
		_ = tx.Rollback()
		t.Fatalf("guarded transition with human override failed: %v", guardErr)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit override tx: %v", err)
	}

	history, err := repo.ListTaskCompletionGateHistory(ctx, task.ID)
	if err != nil {
		t.Fatalf("list history: %v", err)
	}
	var overrideRecord *models.TaskCompletionGateHistory
	for _, h := range history {
		if h.Action == "completion_overridden" {
			overrideRecord = h
			break
		}
	}
	if overrideRecord == nil {
		t.Fatal("expected completion_overridden audit record")
	}
	if overrideRecord.ActorID != "operator-1" || overrideRecord.Reason != "Urgent hotfix release approved out of band" {
		t.Fatalf("override audit record = %+v", overrideRecord)
	}
}

func TestPlanIncrementEvidenceSubjectValidation(t *testing.T) {
	repo := newRepoForCompletionGateTests(t)
	ctx := context.Background()
	task := createTestTaskForGate(t, ctx, repo, "task-increment-validation")
	seedApprovedPlanRevision(t, ctx, repo, task.ID, "rev-val-1")

	snapshot, err := repo.SetTaskCompletionCriteria(ctx, models.TaskCompletionCriteriaChange{
		TaskID:           task.ID,
		WorkspaceID:      task.WorkspaceID,
		ExpectedRevision: 0,
		PlanRevisionID:   "rev-val-1",
		ActorKind:        "agent",
		ActorID:          "architect-agent",
		Criteria: []models.TaskCompletionCriterion{
			{ID: "I1", Description: "Increment 1", EvidenceSubject: models.TaskCompletionEvidenceSubject{Kind: models.TaskCompletionEvidencePlanIncrement, ID: "I1"}},
		},
	})
	if err != nil {
		t.Fatalf("set criteria: %v", err)
	}

	// 1. Wrong revision string ("2" when current is 1)
	_, err = repo.VerifyTaskCompletionCriterion(ctx, models.TaskCompletionEvidenceChange{
		TaskID:           task.ID,
		WorkspaceID:      task.WorkspaceID,
		ExpectedRevision: snapshot.Revision,
		CriterionID:      "I1",
		ActorKind:        "agent",
		ActorID:          "reviewer",
		Evidence: models.TaskCompletionEvidence{
			Subject: models.TaskCompletionEvidenceSubject{
				Kind:     models.TaskCompletionEvidencePlanIncrement,
				ID:       "I1",
				Revision: "2", // wrong revision
			},
		},
	})
	if !errors.Is(err, repoerrors.ErrTaskCompletionEvidenceChanged) {
		t.Fatalf("verify with wrong revision error = %v, want ErrTaskCompletionEvidenceChanged", err)
	}

	// 2. Mismatched subject ID
	_, err = repo.VerifyTaskCompletionCriterion(ctx, models.TaskCompletionEvidenceChange{
		TaskID:           task.ID,
		WorkspaceID:      task.WorkspaceID,
		ExpectedRevision: snapshot.Revision,
		CriterionID:      "I1",
		ActorKind:        "agent",
		ActorID:          "reviewer",
		Evidence: models.TaskCompletionEvidence{
			Subject: models.TaskCompletionEvidenceSubject{
				Kind:     models.TaskCompletionEvidencePlanIncrement,
				ID:       "I2", // mismatch
				Revision: "1",
			},
		},
	})
	if !errors.Is(err, repoerrors.ErrTaskCompletionEvidenceChanged) {
		t.Fatalf("verify with mismatched ID error = %v, want ErrTaskCompletionEvidenceChanged", err)
	}
}

func TestPlanIncrementConcurrentVerification(t *testing.T) {
	repo := newRepoForCompletionGateTests(t)
	ctx := context.Background()
	task := createTestTaskForGate(t, ctx, repo, "task-increment-concurrency")
	seedApprovedPlanRevision(t, ctx, repo, task.ID, "rev-conc-1")

	criteria := make([]models.TaskCompletionCriterion, 10)
	for i := 0; i < 10; i++ {
		id := uuid.NewString()[:8]
		criteria[i] = models.TaskCompletionCriterion{
			ID:              id,
			Description:     "Concurrent Increment " + id,
			EvidenceSubject: models.TaskCompletionEvidenceSubject{Kind: models.TaskCompletionEvidencePlanIncrement, ID: id},
		}
	}
	snapshot, err := repo.SetTaskCompletionCriteria(ctx, models.TaskCompletionCriteriaChange{
		TaskID:           task.ID,
		WorkspaceID:      task.WorkspaceID,
		ExpectedRevision: 0,
		PlanRevisionID:   "rev-conc-1",
		ActorKind:        "agent",
		ActorID:          "architect-agent",
		Criteria:         criteria,
	})
	if err != nil {
		t.Fatalf("set criteria: %v", err)
	}

	var wg sync.WaitGroup
	errCh := make(chan error, len(criteria))

	for _, c := range criteria {
		wg.Add(1)
		go func(crit models.TaskCompletionCriterion) {
			defer wg.Done()
			_, err := repo.VerifyTaskCompletionCriterion(ctx, models.TaskCompletionEvidenceChange{
				TaskID:           task.ID,
				WorkspaceID:      task.WorkspaceID,
				ExpectedRevision: snapshot.Revision,
				CriterionID:      crit.ID,
				ActorKind:        "agent",
				ActorID:          "reviewer-worker",
				Evidence: models.TaskCompletionEvidence{
					Subject: models.TaskCompletionEvidenceSubject{
						Kind:     models.TaskCompletionEvidencePlanIncrement,
						ID:       crit.ID,
						Revision: "1",
					},
					Summary: "Verified in parallel",
				},
			})
			if err != nil {
				errCh <- err
			}
		}(c)
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Errorf("concurrent verification error: %v", err)
	}

	finalSnapshot, err := repo.GetTaskCompletionGate(ctx, task.ID)
	if err != nil {
		t.Fatalf("get final gate: %v", err)
	}
	if finalSnapshot.Blocked {
		t.Fatalf("expected all criteria to be verified, but gate is blocked: %+v", finalSnapshot.Blockers)
	}
}

func TestPlanIncrementApprovalReceiptRequiredForAgentEnrollment(t *testing.T) {
	repo := newRepoForCompletionGateTests(t)
	ctx := context.Background()
	task := createTestTaskForGate(t, ctx, repo, "task-receipt-required")

	// 1. Agent enrollment without plan_revision_id fails
	_, err := repo.SetTaskCompletionCriteria(ctx, models.TaskCompletionCriteriaChange{
		TaskID:           task.ID,
		WorkspaceID:      task.WorkspaceID,
		ExpectedRevision: 0,
		PlanRevisionID:   "",
		ActorKind:        "agent",
		ActorID:          "architect-agent",
		Criteria: []models.TaskCompletionCriterion{
			{ID: "I1", Description: "Increment 1", EvidenceSubject: models.TaskCompletionEvidenceSubject{Kind: models.TaskCompletionEvidencePlanIncrement, ID: "I1"}},
		},
	})
	if !errors.Is(err, repoerrors.ErrTaskCompletionGateBlocked) {
		t.Fatalf("agent enrollment without plan_revision_id err = %v, want ErrTaskCompletionGateBlocked", err)
	}

	// 2. Agent enrollment without approved receipt fails
	_, err = repo.SetTaskCompletionCriteria(ctx, models.TaskCompletionCriteriaChange{
		TaskID:           task.ID,
		WorkspaceID:      task.WorkspaceID,
		ExpectedRevision: 0,
		PlanRevisionID:   "rev-unapproved-1",
		ActorKind:        "agent",
		ActorID:          "architect-agent",
		Criteria: []models.TaskCompletionCriterion{
			{ID: "I1", Description: "Increment 1", EvidenceSubject: models.TaskCompletionEvidenceSubject{Kind: models.TaskCompletionEvidencePlanIncrement, ID: "I1"}},
		},
	})
	if !errors.Is(err, repoerrors.ErrTaskCompletionGateBlocked) {
		t.Fatalf("agent enrollment without approved receipt err = %v, want ErrTaskCompletionGateBlocked", err)
	}

	// 3. Seed unapproved (e.g. revise decision or subject_edited) receipt fails
	now := time.Now().UTC()
	_, _ = repo.db.ExecContext(ctx, `
		INSERT INTO task_plan_approval_receipts (id, task_id, plan_revision_id, write_version, decision, subject_edited, created_at)
		VALUES ('r-rev', ?, 'rev-revise-1', 'wv-1', 'revise', 0, ?)
	`, task.ID, now)
	_, err = repo.SetTaskCompletionCriteria(ctx, models.TaskCompletionCriteriaChange{
		TaskID:           task.ID,
		WorkspaceID:      task.WorkspaceID,
		ExpectedRevision: 0,
		PlanRevisionID:   "rev-revise-1",
		ActorKind:        "agent",
		ActorID:          "architect-agent",
		Criteria: []models.TaskCompletionCriterion{
			{ID: "I1", Description: "Increment 1", EvidenceSubject: models.TaskCompletionEvidenceSubject{Kind: models.TaskCompletionEvidencePlanIncrement, ID: "I1"}},
		},
	})
	if !errors.Is(err, repoerrors.ErrTaskCompletionGateBlocked) {
		t.Fatalf("agent enrollment with revise receipt err = %v, want ErrTaskCompletionGateBlocked", err)
	}

	// 4. Seed approved receipt with matching write versions succeeds
	seedApprovedPlanRevision(t, ctx, repo, task.ID, "rev-valid-1")
	snapshot, err := repo.SetTaskCompletionCriteria(ctx, models.TaskCompletionCriteriaChange{
		TaskID:           task.ID,
		WorkspaceID:      task.WorkspaceID,
		ExpectedRevision: 0,
		PlanRevisionID:   "rev-valid-1",
		ActorKind:        "agent",
		ActorID:          "architect-agent",
		Criteria: []models.TaskCompletionCriterion{
			{ID: "I1", Description: "Increment 1", EvidenceSubject: models.TaskCompletionEvidenceSubject{Kind: models.TaskCompletionEvidencePlanIncrement, ID: "I1"}},
		},
	})
	if err != nil {
		t.Fatalf("agent enrollment with valid receipt err = %v, want nil", err)
	}
	if snapshot.PlanWriteVersion != "wv-rev-valid-1" {
		t.Fatalf("snapshot plan write version = %q, want wv-rev-valid-1", snapshot.PlanWriteVersion)
	}
}

func TestPlanIncrementStalePlanDetection(t *testing.T) {
	repo := newRepoForCompletionGateTests(t)
	ctx := context.Background()
	task := createTestTaskForGate(t, ctx, repo, "task-stale-plan")

	seedApprovedPlanRevision(t, ctx, repo, task.ID, "rev-stale-1")
	snapshot, err := repo.SetTaskCompletionCriteria(ctx, models.TaskCompletionCriteriaChange{
		TaskID:           task.ID,
		WorkspaceID:      task.WorkspaceID,
		ExpectedRevision: 0,
		PlanRevisionID:   "rev-stale-1",
		ActorKind:        "agent",
		ActorID:          "architect-agent",
		Criteria: []models.TaskCompletionCriterion{
			{ID: "I1", Description: "Increment 1", EvidenceSubject: models.TaskCompletionEvidenceSubject{Kind: models.TaskCompletionEvidencePlanIncrement, ID: "I1"}},
		},
	})
	if err != nil {
		t.Fatalf("set criteria: %v", err)
	}
	if snapshot.PlanStale {
		t.Fatal("plan should not be stale immediately after enrollment")
	}

	// Verify criterion I1
	snapshot, err = repo.VerifyTaskCompletionCriterion(ctx, models.TaskCompletionEvidenceChange{
		TaskID:           task.ID,
		WorkspaceID:      task.WorkspaceID,
		ExpectedRevision: snapshot.Revision,
		CriterionID:      "I1",
		ActorKind:        "agent",
		ActorID:          "reviewer",
		Evidence: models.TaskCompletionEvidence{
			Subject: models.TaskCompletionEvidenceSubject{
				Kind:     models.TaskCompletionEvidencePlanIncrement,
				ID:       "I1",
				Revision: "1",
			},
			Summary: "Verified",
		},
	})
	if err != nil {
		t.Fatalf("verify criterion: %v", err)
	}
	if snapshot.Blocked {
		t.Fatal("snapshot should be unblocked after verifying I1")
	}

	// Now modify the plan HEAD (simulating a subsequent plan write)
	_, err = repo.db.ExecContext(ctx, `UPDATE task_plans SET write_version = 'wv-mutated-head' WHERE task_id = ?`, task.ID)
	if err != nil {
		t.Fatalf("mutate plan head: %v", err)
	}

	// Re-read gate: must be PlanStale = true, Blocked = true, with blocker plan_revision_stale
	reloaded, err := repo.GetTaskCompletionGate(ctx, task.ID)
	if err != nil {
		t.Fatalf("get gate after plan mutation: %v", err)
	}
	if !reloaded.PlanStale {
		t.Fatal("expected PlanStale = true after plan HEAD mutation")
	}
	if !reloaded.Blocked {
		t.Fatal("expected Blocked = true after plan HEAD mutation")
	}
	foundBlocker := false
	for _, b := range reloaded.Blockers {
		if b.CriterionID == "plan" && b.Reason == "plan_revision_stale: task plan was modified after criteria enrollment" {
			foundBlocker = true
			break
		}
	}
	if !foundBlocker {
		t.Fatalf("expected plan_revision_stale blocker, got blockers: %+v", reloaded.Blockers)
	}

	// Terminal completion guard must block
	tx, err := repo.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	guardErr := repo.guardTaskCompletionTransitionTx(ctx, tx, task.ID, v1.TaskStateInProgress, v1.TaskStateCompleted, "wf-1", "step-review", "wf-1", "step-done")
	_ = tx.Rollback()
	if !errors.Is(guardErr, repoerrors.ErrTaskCompletionGateBlocked) {
		t.Fatalf("terminal transition guard error on stale plan = %v, want ErrTaskCompletionGateBlocked", guardErr)
	}
}

func TestPlanIncrementUnenrolledPlanGap(t *testing.T) {
	repo := newRepoForCompletionGateTests(t)
	ctx := context.Background()
	task := createTestTaskForGate(t, ctx, repo, "task-unenrolled-gap")

	// Task has approved plan receipt, but 0 enrolled criteria
	now := time.Now().UTC()
	_, err := repo.db.ExecContext(ctx, `
		INSERT INTO task_plan_approval_receipts (id, task_id, plan_revision_id, write_version, decision, subject_edited, created_at)
		VALUES ('r-gap', ?, 'rev-gap-1', 'wv-gap-1', 'approve', 0, ?)
	`, task.ID, now)
	if err != nil {
		t.Fatalf("insert receipt: %v", err)
	}

	gate, err := repo.GetTaskCompletionGate(ctx, task.ID)
	if err != nil {
		t.Fatalf("get gate: %v", err)
	}
	if !gate.Blocked {
		t.Fatal("expected gate to be Blocked = true for unenrolled approved plan")
	}
	foundBlocker := false
	for _, b := range gate.Blockers {
		if b.CriterionID == "plan" && b.Reason == "unenrolled_plan: task plan is approved but completion criteria have not been enrolled" {
			foundBlocker = true
			break
		}
	}
	if !foundBlocker {
		t.Fatalf("expected unenrolled_plan blocker, got: %+v", gate.Blockers)
	}

	// Terminal completion guard must block
	tx, err := repo.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	guardErr := repo.guardTaskCompletionTransitionTx(ctx, tx, task.ID, v1.TaskStateInProgress, v1.TaskStateCompleted, "wf-1", "step-review", "wf-1", "step-done")
	_ = tx.Rollback()
	if !errors.Is(guardErr, repoerrors.ErrTaskCompletionGateBlocked) {
		t.Fatalf("terminal transition guard error on unenrolled plan = %v, want ErrTaskCompletionGateBlocked", guardErr)
	}
}

func TestPlanIncrementScopedWorkflowHandoffAndLegacyPaths(t *testing.T) {
	repo := newRepoForCompletionGateTests(t)
	ctx := context.Background()

	// 1. Scoped workflow task (workflow named "Role Pipeline") without plan or criteria
	now := time.Now().UTC()
	_, err := repo.db.ExecContext(ctx, `
		INSERT INTO workflows (id, workspace_id, name, created_at, updated_at)
		VALUES ('wf-role', 'ws-1', 'Role Pipeline', ?, ?)
	`, now, now)
	if err != nil {
		t.Fatalf("insert workflow: %v", err)
	}
	_, err = repo.db.ExecContext(ctx, `
		INSERT INTO workflow_steps (id, workflow_id, name, prompt, created_at, updated_at)
		VALUES ('step-arch', 'wf-role', 'architect', 'prompt', ?, ?),
		       ('step-impl', 'wf-role', 'implement', 'prompt', ?, ?)
	`, now, now, now, now)
	if err != nil {
		t.Fatalf("insert workflow steps: %v", err)
	}

	scopedTask := &models.Task{
		ID:             "task-scoped-role",
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf-role",
		WorkflowStepID: "step-arch",
		Title:          "Scoped Role Task",
		State:          v1.TaskStateInProgress,
	}
	if err := repo.CreateTask(ctx, scopedTask); err != nil {
		t.Fatalf("create scoped task: %v", err)
	}

	gate, err := repo.GetTaskCompletionGate(ctx, scopedTask.ID)
	if err != nil {
		t.Fatalf("get gate for scoped task: %v", err)
	}
	if !gate.Blocked {
		t.Fatal("expected scoped task without criteria to have Blocked = true")
	}
	foundBlocker := false
	for _, b := range gate.Blockers {
		if b.CriterionID == "plan" && b.Reason == "unenrolled_plan: task plan is required for scoped workflow but completion criteria have not been enrolled" {
			foundBlocker = true
			break
		}
	}
	if !foundBlocker {
		t.Fatalf("expected unenrolled_plan blocker for scoped task, got: %+v", gate.Blockers)
	}

	// Architect to Implement transition must fail closed
	tx, err := repo.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	guardErr := repo.guardTaskCompletionTransitionTx(ctx, tx, scopedTask.ID, v1.TaskStateInProgress, v1.TaskStateInProgress, "wf-role", "step-arch", "wf-role", "step-impl")
	_ = tx.Rollback()
	if !errors.Is(guardErr, repoerrors.ErrTaskCompletionGateBlocked) {
		t.Fatalf("architect-to-implement guard on unenrolled plan = %v, want ErrTaskCompletionGateBlocked", guardErr)
	}

	// Terminal transition must fail closed
	tx, err = repo.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	guardErr = repo.guardTaskCompletionTransitionTx(ctx, tx, scopedTask.ID, v1.TaskStateInProgress, v1.TaskStateCompleted, "wf-role", "step-arch", "wf-role", "step-done")
	_ = tx.Rollback()
	if !errors.Is(guardErr, repoerrors.ErrTaskCompletionGateBlocked) {
		t.Fatalf("terminal transition guard on unenrolled scoped task = %v, want ErrTaskCompletionGateBlocked", guardErr)
	}

	// 2. Legacy non-scoped task (e.g. standard Kanban workflow without role steps or plan)
	_, err = repo.db.ExecContext(ctx, `
		INSERT INTO workflows (id, workspace_id, name, created_at, updated_at)
		VALUES ('wf-legacy', 'ws-1', 'Simple Kanban', ?, ?)
	`, now, now)
	if err != nil {
		t.Fatalf("insert legacy workflow: %v", err)
	}
	_, err = repo.db.ExecContext(ctx, `
		INSERT INTO workflow_steps (id, workflow_id, name, prompt, created_at, updated_at)
		VALUES ('step-backlog', 'wf-legacy', 'backlog', 'prompt', ?, ?),
		       ('step-done', 'wf-legacy', 'done', 'prompt', ?, ?)
	`, now, now, now, now)
	if err != nil {
		t.Fatalf("insert legacy workflow steps: %v", err)
	}

	legacyTask := &models.Task{
		ID:             "task-legacy-kanban",
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf-legacy",
		WorkflowStepID: "step-backlog",
		Title:          "Legacy Task",
		State:          v1.TaskStateInProgress,
	}
	if err := repo.CreateTask(ctx, legacyTask); err != nil {
		t.Fatalf("create legacy task: %v", err)
	}

	legacyGate, err := repo.GetTaskCompletionGate(ctx, legacyTask.ID)
	if err != nil {
		t.Fatalf("get gate for legacy task: %v", err)
	}
	if legacyGate.Blocked {
		t.Fatalf("expected legacy task without criteria to have Blocked = false, got Blockers: %+v", legacyGate.Blockers)
	}
	if len(legacyGate.Blockers) != 0 {
		t.Fatalf("expected legacy task to have 0 blockers, got: %+v", legacyGate.Blockers)
	}

	// Legacy task terminal transition must succeed (not blocked)
	tx, err = repo.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	guardErr = repo.guardTaskCompletionTransitionTx(ctx, tx, legacyTask.ID, v1.TaskStateInProgress, v1.TaskStateCompleted, "wf-legacy", "step-backlog", "wf-legacy", "step-done")
	_ = tx.Rollback()
	if guardErr != nil {
		t.Fatalf("expected legacy task terminal transition to succeed, got: %v", guardErr)
	}
}
