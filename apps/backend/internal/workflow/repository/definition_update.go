package repository

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/workflow/models"
)

type workflowDefinitionSnapshot struct {
	ID, WorkspaceID, Name, Description, Prompt, AgentProfileID, Source, SourcePath string
	UpdatedAt                                                                      time.Time
	Steps                                                                          []*models.WorkflowStep
}

func (r *Repository) WorkflowDefinitionVersion(ctx context.Context, workflowID string) (string, error) {
	snapshot, err := r.loadWorkflowDefinition(ctx, r.ro, workflowID, false)
	if err != nil {
		return "", err
	}
	return workflowDefinitionDigest(snapshot)
}

func (r *Repository) CheckStepRetirement(ctx context.Context, stepID string) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	return r.rejectReferencedStep(ctx, tx, stepID)
}

func workflowDefinitionDigest(snapshot *workflowDefinitionSnapshot) (string, error) {
	data, err := json.Marshal(snapshot)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

func (r *Repository) loadWorkflowDefinition(ctx context.Context, queryer workflowDefinitionQueryer, workflowID string, lock bool) (*workflowDefinitionSnapshot, error) {
	query := `SELECT id, workspace_id, name, COALESCE(description, ''), COALESCE(prompt, ''), COALESCE(agent_profile_id, ''), COALESCE(source, ''), COALESCE(source_path, ''), updated_at FROM workflows WHERE id = ?`
	if lock && dialect.IsPostgres(r.db.DriverName()) {
		query += ` FOR UPDATE`
	}
	var snapshot workflowDefinitionSnapshot
	err := queryer.QueryRowContext(ctx, r.db.Rebind(query), workflowID).Scan(
		&snapshot.ID, &snapshot.WorkspaceID, &snapshot.Name, &snapshot.Description, &snapshot.Prompt,
		&snapshot.AgentProfileID, &snapshot.Source, &snapshot.SourcePath, &snapshot.UpdatedAt,
	)
	if errorsIsNoRows(err) {
		return nil, fmt.Errorf("workflow not found: %s", workflowID)
	}
	if err != nil {
		return nil, err
	}
	rows, err := queryer.QueryContext(ctx, r.db.Rebind(`SELECT `+stepSelectColumns+` FROM workflow_steps WHERE workflow_id = ? AND retired_at IS NULL ORDER BY position, id`), workflowID)
	if err != nil {
		return nil, err
	}
	snapshot.Steps = make([]*models.WorkflowStep, 0)
	for rows.Next() {
		step, scanErr := r.scanStep(rows)
		if scanErr != nil {
			_ = rows.Close()
			return nil, scanErr
		}
		snapshot.Steps = append(snapshot.Steps, step)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	return &snapshot, nil
}

type workflowDefinitionQueryer interface {
	QueryRowContext(context.Context, string, ...interface{}) *sql.Row
	QueryContext(context.Context, string, ...interface{}) (*sql.Rows, error)
}

func errorsIsNoRows(err error) bool { return err == sql.ErrNoRows }

func (r *Repository) ApplyWorkflowDefinition(
	ctx context.Context,
	workflowID, expectedVersion string,
	workflow *taskmodels.Workflow,
	steps []*models.WorkflowStep,
	retiredStepIDs []string,
) (bool, error) {
	return r.applyWorkflowDefinition(ctx, workflowID, expectedVersion, workflow, steps, retiredStepIDs, false, "")
}

func (r *Repository) ApplySyncedWorkflowDefinition(
	ctx context.Context,
	workflowID, expectedVersion, sourcePath string,
	workflow *taskmodels.Workflow,
	steps []*models.WorkflowStep,
	retiredStepIDs []string,
) (bool, error) {
	return r.applyWorkflowDefinition(ctx, workflowID, expectedVersion, workflow, steps, retiredStepIDs, true, sourcePath)
}

func (r *Repository) applyWorkflowDefinition(
	ctx context.Context,
	workflowID, expectedVersion string,
	workflow *taskmodels.Workflow,
	steps []*models.WorkflowStep,
	retiredStepIDs []string,
	allowGitSource bool,
	expectedSourcePath string,
) (bool, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	current, err := r.loadWorkflowDefinition(ctx, tx, workflowID, true)
	if err != nil {
		return false, err
	}
	if err := r.checkWorkflowDefinitionApply(current, expectedVersion, workflow, retiredStepIDs, allowGitSource, expectedSourcePath); err != nil {
		return false, err
	}
	if workflowDefinitionMatches(current, workflow, steps, retiredStepIDs) {
		return false, nil
	}
	for _, stepID := range retiredStepIDs {
		if err := r.rejectReferencedStep(ctx, tx, stepID); err != nil {
			return false, err
		}
	}
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE workflows SET name = ?, description = ?, prompt = ?, agent_profile_id = ?, updated_at = ? WHERE id = ?`),
		workflow.Name, workflow.Description, workflow.Prompt, nullableString(workflow.AgentProfileID), now, workflowID); err != nil {
		return false, err
	}
	if err := writeWorkflowDefinitionSteps(ctx, tx, workflowID, current.Steps, steps, now); err != nil {
		return false, err
	}
	if err := retireWorkflowDefinitionSteps(ctx, tx, workflowID, retiredStepIDs, now); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

// checkWorkflowDefinitionApply enforces the preview's version, the source
// ownership rule, and the identity fields a definition update may not change.
func (r *Repository) checkWorkflowDefinitionApply(
	current *workflowDefinitionSnapshot,
	expectedVersion string,
	workflow *taskmodels.Workflow,
	retiredStepIDs []string,
	allowGitSource bool,
	expectedSourcePath string,
) error {
	version, err := workflowDefinitionDigest(current)
	if err != nil {
		return err
	}
	if expectedVersion == "" || version != expectedVersion {
		return repoerrors.ErrTaskVersionConflict
	}
	if current.Source == taskmodels.WorkflowSourceGitHub && !allowGitSource {
		return fmt.Errorf("git-synced workflows are read-only until released")
	}
	syncedSourceMatches := !allowGitSource ||
		(current.Source == taskmodels.WorkflowSourceGitHub && current.SourcePath == expectedSourcePath)
	if !sameWorkflowIdentity(current, workflow) || !syncedSourceMatches {
		return fmt.Errorf("workflow identity and source ownership cannot change during definition update")
	}
	if len(retiredStepIDs) > 0 && dialect.IsPostgres(r.db.DriverName()) {
		return models.ErrWorkflowStepRetirementUnsupported
	}
	return nil
}

func sameWorkflowIdentity(current *workflowDefinitionSnapshot, workflow *taskmodels.Workflow) bool {
	return workflow != nil && workflow.ID == current.ID && workflow.WorkspaceID == current.WorkspaceID &&
		workflow.Source == current.Source && workflow.SourcePath == current.SourcePath
}

// writeWorkflowDefinitionSteps updates the steps that already exist and
// inserts the new ones, demoting the previous start step first.
func writeWorkflowDefinitionSteps(
	ctx context.Context,
	tx *sqlx.Tx,
	workflowID string,
	current, steps []*models.WorkflowStep,
	now time.Time,
) error {
	existing := make(map[string]bool, len(current))
	for _, step := range current {
		existing[step.ID] = true
	}
	for _, step := range steps {
		if step == nil || step.WorkflowID != workflowID || step.ID == "" {
			return fmt.Errorf("workflow step candidate has invalid identity")
		}
		if step.PortableKey == "" {
			step.PortableKey = uuid.NewString()
		}
		step.UpdatedAt = now
		if !existing[step.ID] {
			if err := insertWorkflowDefinitionStep(ctx, tx, step); err != nil {
				return err
			}
			continue
		}
		if step.IsStartStep {
			if _, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE workflow_steps SET is_start_step = 0, updated_at = ? WHERE workflow_id = ? AND id != ? AND is_start_step = 1 AND retired_at IS NULL`), now, workflowID, step.ID); err != nil {
				return err
			}
		}
		if err := updateWorkflowDefinitionStep(ctx, tx, step); err != nil {
			return err
		}
	}
	return nil
}

// retireWorkflowDefinitionSteps retires each step exactly once; a step that
// is already retired or gone means the preview is stale.
func retireWorkflowDefinitionSteps(ctx context.Context, tx *sqlx.Tx, workflowID string, retiredStepIDs []string, now time.Time) error {
	for _, stepID := range retiredStepIDs {
		result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE workflow_steps SET retired_at = ?, updated_at = ? WHERE id = ? AND workflow_id = ? AND retired_at IS NULL`), now, now, stepID, workflowID)
		if err != nil {
			return err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if rows == 0 {
			return repoerrors.ErrTaskVersionConflict
		}
	}
	return nil
}

func workflowDefinitionMatches(snapshot *workflowDefinitionSnapshot, workflow *taskmodels.Workflow, steps []*models.WorkflowStep, retired []string) bool {
	if workflow.Name != snapshot.Name || workflow.Description != snapshot.Description || workflow.Prompt != snapshot.Prompt || workflow.AgentProfileID != snapshot.AgentProfileID || len(steps) != len(snapshot.Steps) || len(retired) != 0 {
		return false
	}
	existing := make(map[string]*models.WorkflowStep, len(snapshot.Steps))
	for _, step := range snapshot.Steps {
		existing[step.ID] = step
	}
	for _, step := range steps {
		prior := existing[step.ID]
		if prior == nil {
			return false
		}
		candidate := *step
		baseline := *prior
		candidate.CreatedAt, baseline.CreatedAt = time.Time{}, time.Time{}
		candidate.UpdatedAt, baseline.UpdatedAt = time.Time{}, time.Time{}
		candidate.OrderRevision, baseline.OrderRevision = 0, 0
		candidateJSON, err := json.Marshal(candidate)
		if err != nil {
			return false
		}
		baselineJSON, err := json.Marshal(baseline)
		if err != nil || !bytes.Equal(candidateJSON, baselineJSON) {
			return false
		}
	}
	return true
}

func updateWorkflowDefinitionStep(ctx context.Context, tx *sqlx.Tx, step *models.WorkflowStep) error {
	events, err := json.Marshal(step.Events)
	if err != nil {
		return err
	}
	target, err := marshalSessionTarget(step.SessionTarget)
	if err != nil {
		return err
	}
	tags, err := marshalAllowedTags(step.AllowedTags)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`UPDATE workflow_steps SET portable_key = ?, name = ?, position = ?, color = ?, prompt = ?, events = ?, allow_manual_move = ?, is_start_step = ?, show_in_command_panel = ?, auto_archive_after_hours = ?, agent_profile_id = ?, profile_session_start_policy = ?, profile_session_end_policy = ?, disable_unclassified_fallback = ?, stage_type = ?, auto_advance_requires_signal = ?, cancel_triggers_turn_complete = ?, complete_task_on_enter = ?, wip_limit = ?, pull_from_step_id = ?, session_target = ?, allowed_tags = ?, updated_at = ? WHERE id = ? AND workflow_id = ? AND retired_at IS NULL`),
		step.PortableKey, step.Name, step.Position, step.Color, step.Prompt, string(events), dialect.BoolToInt(step.AllowManualMove), dialect.BoolToInt(step.IsStartStep), dialect.BoolToInt(step.ShowInCommandPanel), step.AutoArchiveAfterHours, step.AgentProfileID, taskmodels.NormalizeWorkflowProfileSessionStartPolicy(string(step.ProfileSessionStartPolicy)), taskmodels.NormalizeWorkflowProfileSessionEndPolicy(string(step.ProfileSessionEndPolicy)), dialect.BoolToInt(step.DisableUnclassifiedFallback), normalizeStageType(step.StageType), dialect.BoolToInt(step.AutoAdvanceRequiresSignal), dialect.BoolToInt(step.CancelTriggersTurnComplete), dialect.BoolToInt(step.CompleteTaskOnEnter), step.WIPLimit, step.PullFromStepID, target, tags, step.UpdatedAt, step.ID, step.WorkflowID)
	return err
}

func insertWorkflowDefinitionStep(ctx context.Context, tx *sqlx.Tx, step *models.WorkflowStep) error {
	events, err := json.Marshal(step.Events)
	if err != nil {
		return err
	}
	target, err := marshalSessionTarget(step.SessionTarget)
	if err != nil {
		return err
	}
	tags, err := marshalAllowedTags(step.AllowedTags)
	if err != nil {
		return err
	}
	if step.CreatedAt.IsZero() {
		step.CreatedAt = step.UpdatedAt
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO workflow_steps (id, workflow_id, portable_key, name, position, color, prompt, events, allow_manual_move, is_start_step, show_in_command_panel, auto_archive_after_hours, agent_profile_id, profile_session_start_policy, profile_session_end_policy, disable_unclassified_fallback, stage_type, auto_advance_requires_signal, cancel_triggers_turn_complete, complete_task_on_enter, wip_limit, pull_from_step_id, session_target, allowed_tags, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		step.ID, step.WorkflowID, step.PortableKey, step.Name, step.Position, step.Color, step.Prompt, string(events), dialect.BoolToInt(step.AllowManualMove), dialect.BoolToInt(step.IsStartStep), dialect.BoolToInt(step.ShowInCommandPanel), step.AutoArchiveAfterHours, step.AgentProfileID, taskmodels.NormalizeWorkflowProfileSessionStartPolicy(string(step.ProfileSessionStartPolicy)), taskmodels.NormalizeWorkflowProfileSessionEndPolicy(string(step.ProfileSessionEndPolicy)), dialect.BoolToInt(step.DisableUnclassifiedFallback), normalizeStageType(step.StageType), dialect.BoolToInt(step.AutoAdvanceRequiresSignal), dialect.BoolToInt(step.CancelTriggersTurnComplete), dialect.BoolToInt(step.CompleteTaskOnEnter), step.WIPLimit, step.PullFromStepID, target, tags, step.CreatedAt, step.UpdatedAt)
	return err
}

func nullableString(value string) interface{} {
	if value == "" {
		return nil
	}
	return value
}
