package schema

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
)

func EnsureStepIdentity(writer *sqlx.DB, migrations *db.MigrateLogger) error {
	if err := migrations.Apply("workflow_steps.portable_key", `ALTER TABLE workflow_steps ADD COLUMN portable_key TEXT NOT NULL DEFAULT ''`); err != nil {
		return fmt.Errorf("add portable workflow step key: %w", err)
	}
	if err := migrations.Apply("workflow_steps.retired_at", `ALTER TABLE workflow_steps ADD COLUMN retired_at TIMESTAMP`); err != nil {
		return fmt.Errorf("add workflow step retirement marker: %w", err)
	}
	if err := backfillPortableStepKeys(writer); err != nil {
		return fmt.Errorf("backfill portable workflow step keys: %w", err)
	}
	if err := migrations.Apply("workflow_steps.portable_key_unique", `CREATE UNIQUE INDEX IF NOT EXISTS idx_workflow_steps_portable_key ON workflow_steps(workflow_id, portable_key) WHERE portable_key != ''`); err != nil {
		return fmt.Errorf("index portable workflow step keys: %w", err)
	}
	return nil
}

func backfillPortableStepKeys(writer *sqlx.DB) error {
	tx, err := writer.Beginx()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.Queryx(`SELECT id FROM workflow_steps WHERE portable_key = '' OR portable_key IS NULL`)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := tx.Exec(tx.Rebind(`UPDATE workflow_steps SET portable_key = ? WHERE id = ? AND (portable_key = '' OR portable_key IS NULL)`), uuid.NewString(), id); err != nil {
			return err
		}
	}
	return tx.Commit()
}
