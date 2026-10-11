package sqlite

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

const minorApprovalBaselineContent = "# Plan\n\nIncrement I1.\n\nIncrement I2.\n"

// seedMinorApprovalScenario builds a human-approved plan baseline with an
// enrolled criterion set and returns the current (clarified) plan head content.
func seedMinorApprovalScenario(t *testing.T, ctx context.Context, repo *Repository, taskID string) {
	t.Helper()
	now := time.Now().UTC()
	stepID := "step-arch"
	if _, err := repo.db.ExecContext(ctx, `
		INSERT INTO task_sessions (id, task_id, state, workflow_step_id_at_creation, started_at, updated_at)
		VALUES ('sess-baseline', ?, 'WAITING_FOR_INPUT', ?, ?, ?),
		       ('sess-actor', ?, 'RUNNING', ?, ?, ?)
	`, taskID, stepID, now, now, taskID, stepID, now, now); err != nil {
		t.Fatalf("insert sessions: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx, `
		INSERT INTO task_session_turns (id, task_session_id, task_id, started_at, created_at, updated_at)
		VALUES ('turn-1', 'sess-baseline', ?, ?, ?, ?)
	`, taskID, now, now, now); err != nil {
		t.Fatalf("insert turn: %v", err)
	}
	humanReceiptID := "receipt-human-1"
	if _, err := repo.db.ExecContext(ctx, `
		INSERT INTO task_session_messages (id, task_session_id, task_id, turn_id, author_type, content, requests_input, type, metadata, created_at, updated_at)
		VALUES (?, 'sess-baseline', ?, 'turn-1', 'agent', 'approve?', 1, 'clarification_request', '{}', ?, ?)
	`, humanReceiptID, taskID, now, now); err != nil {
		t.Fatalf("insert clarification message: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx, `
		INSERT INTO task_plans (id, task_id, title, content, created_by, created_at, updated_at, write_version)
		VALUES (?, ?, 'Plan', ?, 'agent', ?, ?, 'wv-1')
	`, "plan-"+taskID, taskID, minorApprovalBaselineContent, now, now); err != nil {
		t.Fatalf("insert plan: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx, `
		INSERT INTO task_plan_revisions (id, task_id, revision_number, title, content, author_kind, author_name, workflow_step_id, created_at, updated_at, write_version)
		VALUES ('rev-1', ?, 1, 'Plan', ?, 'agent', 'architect', ?, ?, ?, 'wv-1')
	`, taskID, minorApprovalBaselineContent, stepID, now, now); err != nil {
		t.Fatalf("insert baseline revision: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx, `
		INSERT INTO task_plan_approval_receipts (id, task_id, plan_revision_id, write_version, decision, subject_edited, kind, created_at)
		VALUES (?, ?, 'rev-1', 'wv-1', 'approve', 0, 'human', ?)
	`, humanReceiptID, taskID, now); err != nil {
		t.Fatalf("insert human receipt: %v", err)
	}
	if _, err := repo.SetTaskCompletionCriteria(ctx, models.TaskCompletionCriteriaChange{
		TaskID: taskID, WorkspaceID: "ws-1", ExpectedRevision: 0, PlanRevisionID: "rev-1",
		ActorKind: "agent", ActorID: "sess-actor",
		Criteria: minorApprovalCriteria(),
	}); err != nil {
		t.Fatalf("enroll baseline criteria: %v", err)
	}
	headContent := minorApprovalBaselineContent + "\n## Architect clarifications (advisory)\n\nClarify I1 wording only.\n"
	if _, err := repo.db.ExecContext(ctx, `
		INSERT INTO task_plan_revisions (id, task_id, revision_number, title, content, author_kind, author_name, workflow_step_id, created_at, updated_at, write_version)
		VALUES ('rev-2', ?, 2, 'Plan', ?, 'agent', 'architect', ?, ?, ?, 'wv-2')
	`, taskID, headContent, stepID, now, now); err != nil {
		t.Fatalf("insert clarified revision: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx, `UPDATE task_plans SET write_version = 'wv-2', content = ? WHERE task_id = ?`, headContent, taskID); err != nil {
		t.Fatalf("update plan head: %v", err)
	}
}

func minorApprovalCriteria() []models.TaskCompletionCriterion {
	return []models.TaskCompletionCriterion{
		{ID: "I1", Description: "Increment 1", EvidenceSubject: models.TaskCompletionEvidenceSubject{Kind: models.TaskCompletionEvidencePlanIncrement, ID: "I1"}},
		{ID: "I2", Description: "Increment 2", EvidenceSubject: models.TaskCompletionEvidenceSubject{Kind: models.TaskCompletionEvidencePlanIncrement, ID: "I2"}},
	}
}

func minorApprovalIncrements() []models.TaskPlanIncrementDeclaration {
	return []models.TaskPlanIncrementDeclaration{
		{ID: "I1", Description: "Increment 1"},
		{ID: "I2", Description: "Increment 2"},
	}
}

func minorApprovalCommand(taskID, headRevisionID string) models.RecordPlanMinorApprovalCommand {
	return models.RecordPlanMinorApprovalCommand{
		TaskID: taskID, ExpectedRevision: 1, ExpectedPlanRevisionID: headRevisionID,
		ActorSessionID: "sess-actor", ActorKind: "agent", ActorID: "sess-actor",
		Summary: "Clarified I1 wording", Attestation: "Non-normative, increments unchanged",
		Increments: minorApprovalIncrements(),
	}
}

func TestRecordPlanMinorApprovalRecordsReceiptAndReenrolls(t *testing.T) {
	repo := newRepoForCompletionGateTests(t)
	ctx := context.Background()
	task := createTestTaskForGate(t, ctx, repo, "task-minor-ok")
	seedMinorApprovalScenario(t, ctx, repo, task.ID)

	result, err := repo.RecordPlanMinorApproval(ctx, minorApprovalCommand(task.ID, "rev-2"))
	if err != nil {
		t.Fatalf("record minor approval: %v", err)
	}
	if result.PlanRevisionID != "rev-2" || result.WriteVersion != "wv-2" {
		t.Fatalf("result = %+v, want rev-2/wv-2", result)
	}
	var kind, baselineRevision, baselineWrite, actorSession, summary string
	if err := repo.db.QueryRowContext(ctx, `
		SELECT kind, baseline_revision_id, baseline_write_version, actor_session_id, summary
		FROM task_plan_approval_receipts WHERE task_id = ? AND plan_revision_id = 'rev-2'
	`, task.ID).Scan(&kind, &baselineRevision, &baselineWrite, &actorSession, &summary); err != nil {
		t.Fatalf("read minor receipt: %v", err)
	}
	if kind != models.TaskPlanApprovalReceiptKindMinor || baselineRevision != "rev-1" ||
		baselineWrite != "wv-1" || actorSession != "sess-actor" || !strings.Contains(summary, "Clarified") {
		t.Fatalf("unexpected receipt: kind=%q baseline=%q/%q actor=%q summary=%q", kind, baselineRevision, baselineWrite, actorSession, summary)
	}
	if result.Gate.PlanStale {
		t.Fatalf("gate after atomic re-enrollment must not be stale: %+v", result.Gate)
	}
	if result.Gate.PlanRevisionID != "rev-2" || len(result.Gate.Criteria) != 2 {
		t.Fatalf("gate after re-enroll = %+v, want rev-2 with 2 criteria", result.Gate)
	}
}

func TestRecordPlanMinorApprovalRollsBackReceiptAndEnrollmentOnCriteriaFailure(t *testing.T) {
	repo := newRepoForCompletionGateTests(t)
	ctx := context.Background()
	task := createTestTaskForGate(t, ctx, repo, "task-minor-rollback")
	seedMinorApprovalScenario(t, ctx, repo, task.ID)
	// A trigger injects a failure after the receipt insert and when the gate
	// revision is advanced. The single repository transaction must roll both back.
	if _, err := repo.db.ExecContext(ctx, `CREATE TRIGGER reject_minor_gate_update BEFORE UPDATE ON task_completion_sets
		WHEN OLD.task_id = '`+task.ID+`' BEGIN SELECT RAISE(ABORT, 'injected enrollment failure'); END`); err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}
	_, err := repo.RecordPlanMinorApproval(ctx, minorApprovalCommand(task.ID, "rev-2"))
	if err == nil {
		t.Fatal("expected injected enrollment failure")
	}
	var receipts int
	if err := repo.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM task_plan_approval_receipts WHERE task_id = ? AND kind = 'minor'`, task.ID).Scan(&receipts); err != nil {
		t.Fatalf("count minor receipts: %v", err)
	}
	if receipts != 0 {
		t.Fatalf("minor receipts = %d, want rollback to zero", receipts)
	}
	gate, err := repo.GetTaskCompletionGate(ctx, task.ID)
	if err != nil {
		t.Fatalf("read gate after rollback: %v", err)
	}
	if gate.Revision != 1 || gate.PlanRevisionID != "rev-1" {
		t.Fatalf("gate after rollback = %+v, want original revision 1 on rev-1", gate)
	}
}

func TestRecordPlanMinorApprovalRejectsMaterialChange(t *testing.T) {
	repo := newRepoForCompletionGateTests(t)
	ctx := context.Background()
	task := createTestTaskForGate(t, ctx, repo, "task-minor-material")
	seedMinorApprovalScenario(t, ctx, repo, task.ID)
	if _, err := repo.db.ExecContext(ctx, `UPDATE task_plan_revisions SET content = '# Plan completely rewritten' WHERE id = 'rev-2'`); err != nil {
		t.Fatalf("mutate revision: %v", err)
	}
	_, err := repo.RecordPlanMinorApproval(ctx, minorApprovalCommand(task.ID, "rev-2"))
	if !errors.Is(err, repoerrors.ErrTaskPlanMinorApprovalNotBaselinePrefix) {
		t.Fatalf("material change err = %v, want ErrTaskPlanMinorApprovalNotBaselinePrefix", err)
	}
}

func TestRecordPlanMinorApprovalRequiresExactlyOneAdvisorySection(t *testing.T) {
	repo := newRepoForCompletionGateTests(t)
	ctx := context.Background()
	task := createTestTaskForGate(t, ctx, repo, "task-minor-two-advice")
	seedMinorApprovalScenario(t, ctx, repo, task.ID)
	double := minorApprovalBaselineContent + "\n## Architect clarifications (advisory)\n\nFirst\n\n## Architect clarifications (advisory)\n\nSecond\n"
	if _, err := repo.db.ExecContext(ctx, `UPDATE task_plan_revisions SET content = ? WHERE id = 'rev-2'`, double); err != nil {
		t.Fatalf("mutate revision: %v", err)
	}
	_, err := repo.RecordPlanMinorApproval(ctx, minorApprovalCommand(task.ID, "rev-2"))
	if !errors.Is(err, repoerrors.ErrTaskPlanMinorApprovalNotBaselinePrefix) {
		t.Fatalf("double section err = %v, want ErrTaskPlanMinorApprovalNotBaselinePrefix", err)
	}
}

func TestRecordPlanMinorApprovalRejectsChangedCriteria(t *testing.T) {
	repo := newRepoForCompletionGateTests(t)
	ctx := context.Background()
	task := createTestTaskForGate(t, ctx, repo, "task-minor-criteria")
	seedMinorApprovalScenario(t, ctx, repo, task.ID)
	command := minorApprovalCommand(task.ID, "rev-2")
	command.Increments[1].Description = "Increment 2 redefined"
	_, err := repo.RecordPlanMinorApproval(ctx, command)
	if !errors.Is(err, repoerrors.ErrTaskPlanMinorApprovalCriteriaChanged) {
		t.Fatalf("changed criteria err = %v, want ErrTaskPlanMinorApprovalCriteriaChanged", err)
	}
}

func TestRecordPlanMinorApprovalRequiresHumanBaseline(t *testing.T) {
	repo := newRepoForCompletionGateTests(t)
	ctx := context.Background()
	task := createTestTaskForGate(t, ctx, repo, "task-minor-no-human")
	seedMinorApprovalScenario(t, ctx, repo, task.ID)
	if _, err := repo.db.ExecContext(ctx, `DELETE FROM task_plan_approval_receipts WHERE task_id = ?`, task.ID); err != nil {
		t.Fatalf("delete receipts: %v", err)
	}
	_, err := repo.RecordPlanMinorApproval(ctx, minorApprovalCommand(task.ID, "rev-2"))
	if !errors.Is(err, repoerrors.ErrTaskPlanMinorApprovalNoHumanBaseline) {
		t.Fatalf("no baseline err = %v, want ErrTaskPlanMinorApprovalNoHumanBaseline", err)
	}
}

func TestRecordPlanMinorApprovalRejectsForeignActor(t *testing.T) {
	repo := newRepoForCompletionGateTests(t)
	ctx := context.Background()
	task := createTestTaskForGate(t, ctx, repo, "task-minor-actor")
	seedMinorApprovalScenario(t, ctx, repo, task.ID)
	if _, err := repo.db.ExecContext(ctx, `UPDATE task_sessions SET workflow_step_id_at_creation = 'step-other' WHERE id = 'sess-actor'`); err != nil {
		t.Fatalf("move actor step: %v", err)
	}
	_, err := repo.RecordPlanMinorApproval(ctx, minorApprovalCommand(task.ID, "rev-2"))
	if !errors.Is(err, repoerrors.ErrTaskPlanMinorApprovalActorNotPlanOwner) {
		t.Fatalf("foreign actor err = %v, want ErrTaskPlanMinorApprovalActorNotPlanOwner", err)
	}
}

func TestRecordPlanMinorApprovalRejectsStaleHead(t *testing.T) {
	repo := newRepoForCompletionGateTests(t)
	ctx := context.Background()
	task := createTestTaskForGate(t, ctx, repo, "task-minor-stale")
	seedMinorApprovalScenario(t, ctx, repo, task.ID)
	_, err := repo.RecordPlanMinorApproval(ctx, minorApprovalCommand(task.ID, "rev-1"))
	if !errors.Is(err, repoerrors.ErrTaskPlanMinorApprovalStaleHead) {
		t.Fatalf("stale head err = %v, want ErrTaskPlanMinorApprovalStaleHead", err)
	}
}

func TestRecordPlanMinorApprovalPreservesVerifiedEvidence(t *testing.T) {
	repo := newRepoForCompletionGateTests(t)
	ctx := context.Background()
	task := createTestTaskForGate(t, ctx, repo, "task-minor-evidence")
	seedMinorApprovalScenario(t, ctx, repo, task.ID)
	gate, err := repo.GetTaskCompletionGate(ctx, task.ID)
	if err != nil {
		t.Fatalf("get gate: %v", err)
	}
	if _, err := repo.VerifyTaskCompletionCriterion(ctx, models.TaskCompletionEvidenceChange{
		TaskID: task.ID, WorkspaceID: "ws-1", ExpectedRevision: gate.Revision,
		CriterionID: "I1", ActorKind: "agent", ActorID: "reviewer",
		Evidence: models.TaskCompletionEvidence{
			Subject: models.TaskCompletionEvidenceSubject{Kind: models.TaskCompletionEvidencePlanIncrement, ID: "I1", Revision: "1"},
			Summary: "Verified I1",
		},
	}); err != nil {
		t.Fatalf("verify I1: %v", err)
	}
	if _, err := repo.RecordPlanMinorApproval(ctx, minorApprovalCommand(task.ID, "rev-2")); err != nil {
		t.Fatalf("record minor approval: %v", err)
	}
	after, err := repo.GetTaskCompletionGate(ctx, task.ID)
	if err != nil {
		t.Fatalf("get gate after: %v", err)
	}
	for _, criterion := range after.Criteria {
		if criterion.ID != "I1" {
			continue
		}
		if criterion.VerifiedRevision != criterion.CriterionRevision || criterion.Evidence == nil ||
			criterion.Evidence.Summary != "Verified I1" {
			t.Fatalf("I1 evidence lost after minor approval: %+v", criterion)
		}
	}
}
