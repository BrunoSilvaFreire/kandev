package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// planMinorAdvisoryHeading is the exact trailing section heading a minor
// clarification may append to a human-approved plan baseline. The server
// enforces this shape because free-text intent cannot be classified.
const planMinorAdvisoryHeading = "## Architect clarifications (advisory)"

// RecordPlanMinorApproval inserts an audited Architect minor-revision approval
// receipt bound to the task's current plan head. It requires a prior human
// approval on the same workflow step, byte-for-byte baseline-prefix content
// with exactly one trailing advisory section, and unchanged criterion shapes.
func (r *Repository) RecordPlanMinorApproval(ctx context.Context, command models.RecordPlanMinorApprovalCommand) (*models.RecordPlanMinorApprovalResult, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	workspaceID, err := r.lockCompletionTask(ctx, tx, command.TaskID)
	if err != nil {
		return nil, err
	}
	gate, err := r.readTaskCompletionGateTx(ctx, tx, command.TaskID, false)
	if err != nil {
		return nil, err
	}
	if gate.Revision != command.ExpectedRevision {
		return nil, repoerrors.ErrTaskCompletionCriteriaConflict
	}
	headRevisionID, headWriteVersion, revisionContent, err := r.readCurrentPlanHeadTx(ctx, tx, command.TaskID, command.ExpectedPlanRevisionID)
	if err != nil {
		return nil, err
	}
	humanSessionID, err := r.humanBaselineSessionTx(ctx, tx, command.TaskID)
	if err != nil {
		return nil, err
	}
	if err := r.requireMinorApprovalActorTx(ctx, tx, command.TaskID, humanSessionID, command.ActorSessionID); err != nil {
		return nil, err
	}
	baselineRevisionID, baselineWriteVersion, baselineContent, err := r.latestApprovedBaselineTx(ctx, tx, command.TaskID)
	if err != nil {
		return nil, err
	}
	if !validMinorPlanClarification(baselineContent, revisionContent) {
		return nil, repoerrors.ErrTaskPlanMinorApprovalNotBaselinePrefix
	}
	if !minorApprovalIncrementsMatchCriteria(command.Increments, gate.Criteria) {
		return nil, repoerrors.ErrTaskPlanMinorApprovalCriteriaChanged
	}
	now := r.nowUTC()
	if err := r.insertMinorApprovalReceiptTx(ctx, tx, command, headRevisionID, headWriteVersion, baselineRevisionID, baselineWriteVersion, now); err != nil {
		return nil, err
	}
	// Re-enroll the unchanged criteria against this exact approved plan head
	// inside the receipt transaction. A failed criteria write rolls back both
	// the receipt and its audit history.
	if err := r.reenrollMinorApprovalCriteriaTx(ctx, tx, command, workspaceID, headRevisionID, headWriteVersion, gate, now); err != nil {
		return nil, err
	}
	if err := insertCompletionHistoryTx(ctx, tx, r.db.Rebind, models.TaskCompletionGateHistory{
		ID: uuid.NewString(), TaskID: command.TaskID, WorkspaceID: workspaceID,
		Revision: gate.Revision, Action: "minor_plan_approved", ActorKind: command.ActorKind,
		ActorID: command.ActorID, Reason: strings.TrimSpace(command.Summary),
		Details: headRevisionID, CreatedAt: now,
	}); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.minorApprovalResult(ctx, command.TaskID, headRevisionID, headWriteVersion)
}

// insertMinorApprovalReceiptTx writes the audited minor-approval receipt bound
// to the exact plan head inside the caller's transaction.
func (r *Repository) insertMinorApprovalReceiptTx(
	ctx context.Context,
	tx *sql.Tx,
	command models.RecordPlanMinorApprovalCommand,
	headRevisionID, headWriteVersion, baselineRevisionID, baselineWriteVersion string,
	now time.Time,
) error {
	_, err := tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_plan_approval_receipts (
			id, task_id, plan_revision_id, write_version, decision, subject_edited,
			kind, baseline_revision_id, baseline_write_version, actor_session_id, summary, attestation, created_at
		) VALUES (?, ?, ?, ?, 'approve', 0, ?, ?, ?, ?, ?, ?, ?)
	`), uuid.NewString(), command.TaskID, headRevisionID, headWriteVersion,
		models.TaskPlanApprovalReceiptKindMinor, baselineRevisionID, baselineWriteVersion,
		command.ActorSessionID, strings.TrimSpace(command.Summary), strings.TrimSpace(command.Attestation), now)
	return err
}

// minorApprovalResult reads the committed gate snapshot for the result envelope.
func (r *Repository) minorApprovalResult(ctx context.Context, taskID, revisionID, writeVersion string) (*models.RecordPlanMinorApprovalResult, error) {
	gate, err := r.GetTaskCompletionGate(ctx, taskID)
	if err != nil {
		return nil, err
	}
	return &models.RecordPlanMinorApprovalResult{
		PlanRevisionID: revisionID, WriteVersion: writeVersion, Gate: gate,
	}, nil
}

func (r *Repository) reenrollMinorApprovalCriteriaTx(
	ctx context.Context,
	tx *sql.Tx,
	command models.RecordPlanMinorApprovalCommand,
	workspaceID, planRevisionID, writeVersion string,
	current *models.TaskCompletionGateSnapshot,
	now time.Time,
) error {
	if err := validateCompletionCriteria(current.Criteria); err != nil {
		return err
	}
	nextRevision := current.Revision + 1
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_completion_sets SET revision = ?, plan_revision_id = ?, plan_write_version = ?, updated_at = ?
		WHERE task_id = ? AND revision = ?
	`), nextRevision, planRevisionID, writeVersion, now, command.TaskID, current.Revision); err != nil {
		return err
	}
	for _, criterion := range current.Criteria {
		if _, err := tx.ExecContext(ctx, r.db.Rebind(`
			UPDATE task_completion_criteria SET verified_revision = ?
			WHERE task_id = ? AND criterion_id = ? AND criterion_revision = ?
		`), criterion.VerifiedRevision, command.TaskID, criterion.ID, criterion.CriterionRevision); err != nil {
			return err
		}
	}
	details, _ := json.Marshal(current.Criteria)
	return insertCompletionHistoryTx(ctx, tx, r.db.Rebind, models.TaskCompletionGateHistory{
		ID: uuid.NewString(), TaskID: command.TaskID, WorkspaceID: workspaceID,
		Revision: nextRevision, Action: "criteria_enrolled", ActorKind: command.ActorKind,
		ActorID: command.ActorID, Details: string(details), CreatedAt: now,
	})
}

// readCurrentPlanHeadTx reads the task's current plan head and its latest
// revision, requiring the caller's asserted revision (when present) to match.
func (r *Repository) readCurrentPlanHeadTx(ctx context.Context, tx *sql.Tx, taskID, expectedRevisionID string) (revisionID, writeVersion, content string, err error) {
	if err := tx.QueryRowContext(ctx, r.db.Rebind(`
		SELECT write_version FROM task_plans WHERE task_id = ?
	`), taskID).Scan(&writeVersion); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", "", "", repoerrors.ErrTaskPlanMinorApprovalNoHumanBaseline
		}
		return "", "", "", err
	}
	var revisionWriteVersion string
	err = tx.QueryRowContext(ctx, r.db.Rebind(`
		SELECT id, content, write_version FROM task_plan_revisions
		WHERE task_id = ? ORDER BY revision_number DESC LIMIT 1
	`), taskID).Scan(&revisionID, &content, &revisionWriteVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", "", repoerrors.ErrTaskPlanMinorApprovalNoHumanBaseline
	}
	if err != nil {
		return "", "", "", err
	}
	if strings.TrimSpace(expectedRevisionID) != "" && expectedRevisionID != revisionID {
		return "", "", "", repoerrors.ErrTaskPlanMinorApprovalStaleHead
	}
	if revisionWriteVersion != writeVersion {
		return "", "", "", repoerrors.ErrTaskPlanMinorApprovalStaleHead
	}
	return revisionID, writeVersion, content, nil
}

// humanBaselineSessionTx returns the session that requested the winning human
// approval. A minor self-approval is only valid from a session in the same
// workflow step as this authority.
func (r *Repository) humanBaselineSessionTx(ctx context.Context, tx *sql.Tx, taskID string) (string, error) {
	var receiptID string
	err := tx.QueryRowContext(ctx, r.db.Rebind(`
		SELECT id FROM task_plan_approval_receipts
		WHERE task_id = ? AND decision = 'approve' AND subject_edited = 0 AND kind = 'human'
		ORDER BY created_at DESC, id DESC LIMIT 1
	`), taskID).Scan(&receiptID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", repoerrors.ErrTaskPlanMinorApprovalNoHumanBaseline
	}
	if err != nil {
		return "", err
	}
	var sessionID string
	// A human approval receipt is keyed by the clarification message id, so the
	// originating session is the message's task_session_id.
	err = tx.QueryRowContext(ctx, r.db.Rebind(`
		SELECT task_session_id FROM task_session_messages WHERE id = ?
	`), receiptID).Scan(&sessionID)
	if errors.Is(err, sql.ErrNoRows) || strings.TrimSpace(sessionID) == "" {
		return "", repoerrors.ErrTaskPlanMinorApprovalActorNotPlanOwner
	}
	if err != nil {
		return "", err
	}
	var baselineStepID string
	if err := tx.QueryRowContext(ctx, r.db.Rebind(`
		SELECT workflow_step_id_at_creation FROM task_sessions WHERE id = ? AND task_id = ?
	`), sessionID, taskID).Scan(&baselineStepID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", repoerrors.ErrTaskPlanMinorApprovalActorNotPlanOwner
		}
		return "", err
	}
	if strings.TrimSpace(baselineStepID) == "" {
		return "", repoerrors.ErrTaskPlanMinorApprovalActorNotPlanOwner
	}
	return sessionID, nil
}

// requireMinorApprovalActorTx verifies the calling session belongs to the task,
// is active, and runs in the same workflow step as the human baseline authority.
func (r *Repository) requireMinorApprovalActorTx(ctx context.Context, tx *sql.Tx, taskID, baselineSessionID, actorSessionID string) error {
	if strings.TrimSpace(actorSessionID) == "" {
		return repoerrors.ErrTaskPlanMinorApprovalActorNotPlanOwner
	}
	var sessionTaskID, state, stepID string
	err := tx.QueryRowContext(ctx, r.db.Rebind(`
		SELECT task_id, state, workflow_step_id_at_creation FROM task_sessions WHERE id = ?
	`), actorSessionID).Scan(&sessionTaskID, &state, &stepID)
	if errors.Is(err, sql.ErrNoRows) {
		return repoerrors.ErrTaskPlanMinorApprovalActorNotPlanOwner
	}
	if err != nil {
		return err
	}
	if sessionTaskID != taskID || !isActiveMinorApprovalSessionState(state) {
		return repoerrors.ErrTaskPlanMinorApprovalActorNotPlanOwner
	}
	var baselineStepID string
	if err := tx.QueryRowContext(ctx, r.db.Rebind(`
		SELECT workflow_step_id_at_creation FROM task_sessions WHERE id = ? AND task_id = ?
	`), baselineSessionID, taskID).Scan(&baselineStepID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return repoerrors.ErrTaskPlanMinorApprovalActorNotPlanOwner
		}
		return err
	}
	if strings.TrimSpace(stepID) == "" || stepID != strings.TrimSpace(baselineStepID) {
		return repoerrors.ErrTaskPlanMinorApprovalActorNotPlanOwner
	}
	return nil
}

// latestApprovedBaselineTx returns the most recent enrollable approval's
// revision identity and content, which the new head must extend verbatim.
func (r *Repository) latestApprovedBaselineTx(ctx context.Context, tx *sql.Tx, taskID string) (revisionID, writeVersion, content string, err error) {
	err = tx.QueryRowContext(ctx, r.db.Rebind(`
		SELECT plan_revision_id, write_version FROM task_plan_approval_receipts
		WHERE task_id = ? AND decision = 'approve' AND subject_edited = 0
		ORDER BY created_at DESC, id DESC LIMIT 1
	`), taskID).Scan(&revisionID, &writeVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", "", repoerrors.ErrTaskPlanMinorApprovalNoHumanBaseline
	}
	if err != nil {
		return "", "", "", err
	}
	if err := tx.QueryRowContext(ctx, r.db.Rebind(`
		SELECT content FROM task_plan_revisions WHERE id = ? AND task_id = ?
	`), revisionID, taskID).Scan(&content); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", "", "", repoerrors.ErrTaskPlanMinorApprovalNoHumanBaseline
		}
		return "", "", "", err
	}
	return revisionID, writeVersion, content, nil
}

func isActiveMinorApprovalSessionState(state string) bool {
	switch models.TaskSessionState(state) {
	case models.TaskSessionStateCreated, models.TaskSessionStateStarting,
		models.TaskSessionStateRunning, models.TaskSessionStateWaitingForInput:
		return true
	default:
		return false
	}
}

// validMinorPlanClarification enforces the server-checkable "minor" shape: the
// new content is the prior approved content verbatim, followed only by exactly
// one trailing advisory section. Any other diff requires human approval.
func validMinorPlanClarification(previous, next string) bool {
	if previous == "" || next == previous || !strings.HasPrefix(next, previous) {
		return false
	}
	suffix := strings.TrimLeft(next[len(previous):], "\r\n")
	if !strings.HasPrefix(suffix, planMinorAdvisoryHeading) {
		return false
	}
	return strings.Count(next, planMinorAdvisoryHeading) == strings.Count(previous, planMinorAdvisoryHeading)+1
}

// minorApprovalIncrementsMatchCriteria requires unchanged criterion IDs,
// descriptions, and plan-increment evidence subjects.
func minorApprovalIncrementsMatchCriteria(increments []models.TaskPlanIncrementDeclaration, criteria []models.TaskCompletionCriterion) bool {
	if len(increments) != len(criteria) {
		return false
	}
	byID := make(map[string]models.TaskCompletionCriterion, len(criteria))
	for _, criterion := range criteria {
		byID[criterion.ID] = criterion
	}
	seen := make(map[string]struct{}, len(increments))
	for _, increment := range increments {
		id := strings.TrimSpace(increment.ID)
		description := strings.TrimSpace(increment.Description)
		if id == "" || description == "" {
			return false
		}
		if _, duplicate := seen[id]; duplicate {
			return false
		}
		seen[id] = struct{}{}
		criterion, ok := byID[id]
		if !ok {
			return false
		}
		if strings.TrimSpace(criterion.Description) != description {
			return false
		}
		if criterion.EvidenceSubject.Kind != models.TaskCompletionEvidencePlanIncrement ||
			strings.TrimSpace(criterion.EvidenceSubject.ID) != id {
			return false
		}
	}
	return true
}
