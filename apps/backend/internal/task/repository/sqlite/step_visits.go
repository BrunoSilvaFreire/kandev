package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
)

func (r *Repository) initStepVisitsSchema() error {
	if err := r.migrate.Apply("task_step_visits.tables", `
		CREATE TABLE IF NOT EXISTS task_step_visits (
			visit_id TEXT NOT NULL,
			task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
			workflow_id TEXT NOT NULL DEFAULT '',
			step_id TEXT NOT NULL,
			entry_ordinal BIGINT NOT NULL,
			entered_at TIMESTAMP,
			source_transition_id BIGINT UNIQUE,
			provenance TEXT NOT NULL CHECK (provenance IN ('transition', 'activation_baseline')),
			PRIMARY KEY (task_id, visit_id),
			UNIQUE (task_id, entry_ordinal)
		);
		CREATE INDEX IF NOT EXISTS idx_task_step_visits_step
			ON task_step_visits(task_id, step_id, entry_ordinal);
	`); err != nil {
		return fmt.Errorf("create task step visit table: %w", err)
	}
	if _, err := r.db.ExecContext(r.migrationContext(), `CREATE UNIQUE INDEX IF NOT EXISTS uniq_task_sessions_task_id_id ON task_sessions(task_id, id)`); err != nil {
		return fmt.Errorf("index task session identity for visit associations: %w", err)
	}
	if err := r.migrate.Apply("task_session_step_visits.table", `
		CREATE TABLE IF NOT EXISTS task_session_step_visits (
			session_id TEXT NOT NULL,
			task_id TEXT NOT NULL,
			visit_id TEXT NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			PRIMARY KEY (session_id),
			FOREIGN KEY (task_id, session_id) REFERENCES task_sessions(task_id, id) ON DELETE CASCADE,
			FOREIGN KEY (task_id, visit_id) REFERENCES task_step_visits(task_id, visit_id) ON DELETE CASCADE
		);
		CREATE INDEX IF NOT EXISTS idx_task_session_step_visits_task
			ON task_session_step_visits(task_id, visit_id);
	`); err != nil {
		return fmt.Errorf("create task session visit association table: %w", err)
	}
	if err := r.backfillTaskStepVisits(); err != nil {
		return err
	}
	if err := r.migrate.Err(); err != nil {
		return fmt.Errorf("required task step visit migration: %w", err)
	}
	return nil
}

func (r *Repository) backfillTaskStepVisits() error {
	tx, err := r.db.BeginTxx(r.migrationContext(), nil)
	if err != nil {
		return fmt.Errorf("begin task step visit migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryxContext(r.migrationContext(), `
		SELECT id, task_id, COALESCE(to_workflow_id, ''), to_workflow_step_id, occurred_at
		FROM task_step_transitions
		WHERE to_workflow_step_id IS NOT NULL AND to_workflow_step_id <> ''
		ORDER BY task_id, id
	`)
	if err != nil {
		return fmt.Errorf("read recorded step destinations: %w", err)
	}
	for rows.Next() {
		var transitionID int64
		var taskID, workflowID, stepID string
		var enteredAt time.Time
		if err := rows.Scan(&transitionID, &taskID, &workflowID, &stepID, &enteredAt); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan recorded step destination: %w", err)
		}
		visitID := fmt.Sprintf("transition:%d", transitionID)
		if _, err := tx.ExecContext(r.migrationContext(), r.db.Rebind(`
			INSERT INTO task_step_visits (visit_id, task_id, workflow_id, step_id, entry_ordinal, entered_at, source_transition_id, provenance)
			SELECT ?, ?, ?, ?, COALESCE(MAX(entry_ordinal), 0) + 1, ?, ?, 'transition'
			FROM task_step_visits WHERE task_id = ?
			ON CONFLICT DO NOTHING
		`), visitID, taskID, workflowID, stepID, enteredAt, transitionID, taskID); err != nil {
			_ = rows.Close()
			return fmt.Errorf("materialize recorded step visit: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("iterate recorded step destinations: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close recorded step destinations: %w", err)
	}
	baselineID := uuid.NewString()
	//nolint:dupword // adjacent NULL column values in the SQL literal.
	if _, err := tx.ExecContext(r.migrationContext(), r.db.Rebind(`
		INSERT INTO task_step_visits (visit_id, task_id, workflow_id, step_id, entry_ordinal, entered_at, source_transition_id, provenance)
		SELECT ?, t.id, COALESCE(t.workflow_id, ''), t.workflow_step_id,
			COALESCE((SELECT MAX(v.entry_ordinal) FROM task_step_visits v WHERE v.task_id = t.id), 0) + 1,
			NULL, NULL, 'activation_baseline'
		FROM tasks t
		WHERE COALESCE(t.workflow_step_id, '') <> ''
			AND NOT EXISTS (SELECT 1 FROM task_step_visits v WHERE v.task_id = t.id AND v.step_id = t.workflow_step_id)
		ON CONFLICT DO NOTHING
	`), baselineID); err != nil {
		return fmt.Errorf("create labelled step activation baseline: %w", err)
	}
	if err := backfillUnambiguousSessionVisitAssociations(r.migrationContext(), tx, r.db); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit task step visit migration: %w", err)
	}
	return nil
}

func backfillUnambiguousSessionVisitAssociations(ctx context.Context, tx *sqlx.Tx, db *sqlx.DB) error {
	query := `
		INSERT INTO task_session_step_visits (session_id, task_id, visit_id, updated_at)
		SELECT ts.id, ts.task_id, candidates.visit_id, CURRENT_TIMESTAMP
		FROM task_sessions ts
		JOIN (
			SELECT session_id, visit_id FROM (
				SELECT ts.id AS session_id, v.visit_id,
					COUNT(*) OVER (PARTITION BY ts.id) AS matches
				FROM task_sessions ts
				JOIN task_step_visits v ON v.task_id = ts.task_id AND v.provenance = 'transition'
				WHERE ts.workflow_step_id_at_creation = v.step_id
					AND ts.started_at >= v.entered_at
					AND NOT EXISTS (
						SELECT 1 FROM task_step_transitions later
						WHERE later.task_id = ts.task_id
							AND later.id > v.source_transition_id
							AND later.occurred_at <= ts.started_at
					)
					AND NOT EXISTS (
						SELECT 1 FROM task_session_routes route
						WHERE route.destination_session_id = ts.id
							AND route.destination_workflow_step_id <> v.step_id
					)
			) matches WHERE matches = 1
		) candidates ON candidates.session_id = ts.id
		WHERE NOT EXISTS (SELECT 1 FROM task_session_step_visits assoc WHERE assoc.session_id = ts.id)
	`
	if _, err := tx.ExecContext(ctx, db.Rebind(query)); err != nil {
		return fmt.Errorf("associate sessions with uniquely evidenced step visits: %w", err)
	}
	return nil
}

func (r *Repository) associateSessionWithCurrentVisitTx(ctx context.Context, tx stepTransitionTx, taskID, sessionID string, now time.Time) error {
	var visitID sql.NullString
	query := `SELECT v.visit_id FROM tasks t
		JOIN task_step_visits v ON v.task_id = t.id AND v.step_id = t.workflow_step_id
		WHERE t.id = ? AND COALESCE(t.workflow_step_id, '') <> ''
		ORDER BY v.entry_ordinal DESC LIMIT 1`
	if err := tx.QueryRowContext(ctx, r.db.Rebind(query), taskID).Scan(&visitID); err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return fmt.Errorf("read current task step visit: %w", err)
	}
	if !visitID.Valid {
		return nil
	}
	_, err := tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_session_step_visits (session_id, task_id, visit_id, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (session_id) DO UPDATE SET task_id = excluded.task_id, visit_id = excluded.visit_id, updated_at = excluded.updated_at
	`), sessionID, taskID, visitID.String, now)
	if err != nil {
		return fmt.Errorf("associate session with current task visit: %w", err)
	}
	return nil
}

func (r *Repository) ListTaskStepVisits(ctx context.Context, taskID string) ([]models.StepVisitSummary, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT v.visit_id, v.step_id, COALESCE(ws.name, v.step_id), v.entry_ordinal, v.entered_at, v.provenance,
			CASE WHEN ws.id IS NOT NULL AND ws.retired_at IS NULL THEN 1 ELSE 0 END,
			s.id, COALESCE(s.agent_profile_id, ''), s.started_at
		FROM task_step_visits v
		LEFT JOIN workflow_steps ws ON ws.id = v.step_id AND ws.workflow_id = v.workflow_id
		LEFT JOIN task_session_step_visits a ON a.task_id = v.task_id AND a.visit_id = v.visit_id
		LEFT JOIN task_sessions s ON s.id = a.session_id
		WHERE v.task_id = ?
		ORDER BY v.entry_ordinal, a.updated_at, s.id
	`), taskID)
	if err != nil {
		return nil, fmt.Errorf("list task step visits: %w", err)
	}
	defer func() { _ = rows.Close() }()
	byVisit := make([]models.StepVisitSummary, 0)
	indexes := make(map[string]int)
	for rows.Next() {
		var visitID, stepID, stepName, provenance string
		var ordinal int64
		var enteredAt sql.NullTime
		var eligible bool
		var sessionID, profileID sql.NullString
		var associatedAt sql.NullTime
		if err := rows.Scan(&visitID, &stepID, &stepName, &ordinal, &enteredAt, &provenance, &eligible, &sessionID, &profileID, &associatedAt); err != nil {
			return nil, fmt.Errorf("scan task step visit: %w", err)
		}
		index, ok := indexes[visitID]
		if !ok {
			index = len(byVisit)
			indexes[visitID] = index
			visit := models.StepVisitSummary{VisitID: visitID, StepID: stepID, StepName: stepName, Count: 1, Ordinal: ordinal, Provenance: provenance, Eligible: eligible, Sessions: []models.StepVisitSession{}}
			if enteredAt.Valid {
				visit.EnteredAt = &enteredAt.Time
			}
			byVisit = append(byVisit, visit)
		}
		if sessionID.Valid {
			byVisit[index].Sessions = append(byVisit[index].Sessions, models.StepVisitSession{SessionID: sessionID.String, AgentProfileID: profileID.String, OccurredAt: associatedAt.Time})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate task step visits: %w", err)
	}
	return byVisit, nil
}

func (r *Repository) ReassignSessionStepVisit(ctx context.Context, taskID, sessionID, visitID, expectedVisitID string, now time.Time) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, _, found, err := r.readTaskStepInTx(ctx, tx, taskID); err != nil {
		return err
	} else if !found {
		return fmt.Errorf("task not found")
	}
	var currentVisit sql.NullString
	if err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT visit_id FROM task_session_step_visits WHERE task_id = ? AND session_id = ?`), taskID, sessionID).Scan(&currentVisit); err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("read session visit association: %w", err)
	}
	current := ""
	if currentVisit.Valid {
		current = currentVisit.String
	}
	if current != expectedVisitID {
		return ErrStepVisitConflict
	}
	if visitID == "" {
		if _, err := tx.ExecContext(ctx, r.db.Rebind(`DELETE FROM task_session_step_visits WHERE task_id = ? AND session_id = ?`), taskID, sessionID); err != nil {
			return fmt.Errorf("clear session step visit association: %w", err)
		}
		return tx.Commit()
	}
	if err := r.requireEligibleStepVisit(ctx, tx, taskID, visitID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_session_step_visits (session_id, task_id, visit_id, updated_at) VALUES (?, ?, ?, ?)
		ON CONFLICT (session_id) DO UPDATE SET visit_id = excluded.visit_id, updated_at = excluded.updated_at
	`), sessionID, taskID, visitID, now); err != nil {
		return fmt.Errorf("reassign session step visit: %w", err)
	}
	return tx.Commit()
}

// requireEligibleStepVisit rejects a visit that is not the task's, whose step
// row is gone, or whose step is retired.
func (r *Repository) requireEligibleStepVisit(ctx context.Context, tx *sqlx.Tx, taskID, visitID string) error {
	var stepID string
	var existingStepID sql.NullString
	var retiredAt sql.NullString
	query := `SELECT v.step_id, ws.id, ws.retired_at FROM task_step_visits v
		LEFT JOIN workflow_steps ws ON ws.workflow_id = v.workflow_id AND ws.id = v.step_id
		WHERE v.task_id = ? AND v.visit_id = ?`
	if err := tx.QueryRowContext(ctx, r.db.Rebind(query), taskID, visitID).Scan(&stepID, &existingStepID, &retiredAt); err != nil {
		if err == sql.ErrNoRows {
			return models.ErrStepVisitNotFound
		}
		return fmt.Errorf("validate step visit eligibility: %w", err)
	}
	if !existingStepID.Valid {
		return ErrStepVisitNotFound
	}
	if retiredAt.Valid {
		return ErrStepVisitRetired
	}
	return nil
}
