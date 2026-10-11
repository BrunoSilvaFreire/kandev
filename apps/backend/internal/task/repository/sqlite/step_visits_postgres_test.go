package sqlite

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresStepVisitMigrationReplayDoesNotWriteLedger(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize postgres schema: %v", err)
	}
	now := time.Now().UTC()
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO tasks (id, workspace_id, workflow_id, workflow_step_id, title, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`), "task-step-visits-pg", "ws-step-visits-pg", "wf-step-visits-pg", "step-a", "Visit task", now, now); err != nil {
		t.Fatalf("seed current task: %v", err)
	}
	if err := repo.initStepVisitsSchema(); err != nil {
		t.Fatalf("replay postgres visit migration: %v", err)
	}
	var visits, transitions int
	if err := db.Get(&visits, db.Rebind(`SELECT COUNT(*) FROM task_step_visits WHERE task_id = ?`), "task-step-visits-pg"); err != nil {
		t.Fatal(err)
	}
	if err := db.Get(&transitions, db.Rebind(`SELECT COUNT(*) FROM task_step_transitions WHERE task_id = ?`), "task-step-visits-pg"); err != nil {
		t.Fatal(err)
	}
	if visits != 1 || transitions != 0 {
		t.Fatalf("postgres migration visits/transitions = %d/%d, want activation baseline and no ledger rows", visits, transitions)
	}
}
