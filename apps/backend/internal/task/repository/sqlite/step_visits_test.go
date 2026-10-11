package sqlite

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestStepVisitsPersistReentryAndAssociateNewSession(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	ctx := context.Background()
	task := createStepTransitionsTestTask(t, repo, "task-visits", "wf-visits", "step-a")
	task.WorkflowStepID = "step-b"
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("move to step-b: %v", err)
	}
	task.WorkflowStepID = "step-a"
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("re-enter step-a: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session-visits", TaskID: task.ID}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	var associatedVisit string
	if err := repo.db.Get(&associatedVisit, `SELECT visit_id FROM task_session_step_visits WHERE task_id = ? AND session_id = ?`, task.ID, "session-visits"); err != nil {
		t.Fatalf("read session visit association: %v", err)
	}
	var visitCount int
	if err := repo.db.Get(&visitCount, `SELECT COUNT(*) FROM task_step_visits WHERE task_id = ? AND step_id = 'step-a'`, task.ID); err != nil {
		t.Fatalf("count step-a visits: %v", err)
	}
	if visitCount != 2 {
		t.Fatalf("step-a visit count = %d, want 2", visitCount)
	}
	var latestVisit string
	if err := repo.db.Get(&latestVisit, `SELECT visit_id FROM task_step_visits WHERE task_id = ? AND step_id = 'step-a' ORDER BY entry_ordinal DESC LIMIT 1`, task.ID); err != nil {
		t.Fatalf("read latest step-a visit: %v", err)
	}
	if associatedVisit != latestVisit {
		t.Fatalf("session associated visit = %q, want current visit %q", associatedVisit, latestVisit)
	}
	loaded, err := repo.GetTaskSession(ctx, "session-visits")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	if loaded.AssociatedStepVisitID != latestVisit {
		t.Fatalf("session payload associated visit = %q, want %q", loaded.AssociatedStepVisitID, latestVisit)
	}
	visits, err := repo.ListTaskStepVisits(ctx, task.ID)
	if err != nil {
		t.Fatalf("list visits: %v", err)
	}
	var associatedSessionFound bool
	for _, visit := range visits {
		for _, visitSession := range visit.Sessions {
			if visitSession.SessionID == "session-visits" {
				associatedSessionFound = true
				if !visitSession.OccurredAt.Equal(loaded.StartedAt) {
					t.Fatalf("visit session timestamp = %v, want session start %v", visitSession.OccurredAt, loaded.StartedAt)
				}
			}
		}
	}
	if !associatedSessionFound {
		t.Fatal("session is missing from its associated visit")
	}
}

func TestStepVisitMigrationCreatesActivationBaselineWithoutLedgerOrLegacyAssociation(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	ctx := context.Background()
	task := createStepTransitionsTestTask(t, repo, "task-visit-baseline", "wf-baseline", "step-current")
	started := time.Now().UTC().Add(-time.Hour)
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "legacy-session", TaskID: task.ID, StartedAt: started, WorkflowStepIDAtCreation: "step-current"}); err != nil {
		t.Fatalf("create legacy session: %v", err)
	}
	if _, err := repo.db.Exec(`DELETE FROM task_session_step_visits WHERE task_id = ?`, task.ID); err != nil {
		t.Fatalf("clear associations for legacy fixture: %v", err)
	}
	if _, err := repo.db.Exec(`DELETE FROM task_step_visits WHERE task_id = ?`, task.ID); err != nil {
		t.Fatalf("clear visits for legacy fixture: %v", err)
	}
	if _, err := repo.db.Exec(`DELETE FROM task_step_transitions WHERE task_id = ?`, task.ID); err != nil {
		t.Fatalf("clear transition rows for legacy fixture: %v", err)
	}
	if err := repo.initStepVisitsSchema(); err != nil {
		t.Fatalf("run visit migration: %v", err)
	}
	var baseline struct {
		VisitID    string       `db:"visit_id"`
		Provenance string       `db:"provenance"`
		EnteredAt  sql.NullTime `db:"entered_at"`
	}
	if err := repo.db.Get(&baseline, `SELECT visit_id, provenance, entered_at FROM task_step_visits WHERE task_id = ?`, task.ID); err != nil {
		t.Fatalf("read activation baseline: %v", err)
	}
	if baseline.Provenance != "activation_baseline" || baseline.EnteredAt.Valid {
		t.Fatalf("baseline = %+v, want activation_baseline with unknown entry time", baseline)
	}
	var transitionCount, associationCount int
	if err := repo.db.Get(&transitionCount, `SELECT COUNT(*) FROM task_step_transitions WHERE task_id = ?`, task.ID); err != nil {
		t.Fatalf("count transitions: %v", err)
	}
	if err := repo.db.Get(&associationCount, `SELECT COUNT(*) FROM task_session_step_visits WHERE task_id = ?`, task.ID); err != nil {
		t.Fatalf("count associations: %v", err)
	}
	if transitionCount != 0 || associationCount != 0 {
		t.Fatalf("migration wrote %d transition rows and %d legacy associations, want neither", transitionCount, associationCount)
	}
}

func TestStepVisitMigrationReplayPreservesProjection(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	ctx := context.Background()
	task := createStepTransitionsTestTask(t, repo, "task-visit-replay", "wf-replay", "step-a")
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("replay unchanged task: %v", err)
	}
	var beforeVisits, beforeTransitions int
	if err := repo.db.Get(&beforeVisits, `SELECT COUNT(*) FROM task_step_visits WHERE task_id = ?`, task.ID); err != nil {
		t.Fatal(err)
	}
	if err := repo.db.Get(&beforeTransitions, `SELECT COUNT(*) FROM task_step_transitions WHERE task_id = ?`, task.ID); err != nil {
		t.Fatal(err)
	}
	if err := repo.initStepVisitsSchema(); err != nil {
		t.Fatalf("replay visit migration: %v", err)
	}
	var afterVisits, afterTransitions int
	if err := repo.db.Get(&afterVisits, `SELECT COUNT(*) FROM task_step_visits WHERE task_id = ?`, task.ID); err != nil {
		t.Fatal(err)
	}
	if err := repo.db.Get(&afterTransitions, `SELECT COUNT(*) FROM task_step_transitions WHERE task_id = ?`, task.ID); err != nil {
		t.Fatal(err)
	}
	if afterVisits != beforeVisits || afterTransitions != beforeTransitions {
		t.Fatalf("migration replay changed visits/transitions: %d/%d -> %d/%d", beforeVisits, beforeTransitions, afterVisits, afterTransitions)
	}
}

func TestReassignSessionStepVisitRejectsVisitOwnedByAnotherTask(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	ctx := context.Background()
	first := createStepTransitionsTestTask(t, repo, "task-visit-first", "wf-1", "step-a")
	second := createStepTransitionsTestTask(t, repo, "task-visit-second", "wf-1", "step-b")
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session-visit-first", TaskID: first.ID}); err != nil {
		t.Fatalf("create first session: %v", err)
	}
	var foreignVisit string
	if err := repo.db.Get(&foreignVisit, `SELECT visit_id FROM task_step_visits WHERE task_id = ?`, second.ID); err != nil {
		t.Fatalf("read foreign visit: %v", err)
	}
	var currentVisit string
	if err := repo.db.Get(&currentVisit, `SELECT visit_id FROM task_session_step_visits WHERE task_id = ? AND session_id = ?`, first.ID, "session-visit-first"); err != nil {
		t.Fatalf("read current visit: %v", err)
	}
	err := repo.ReassignSessionStepVisit(ctx, first.ID, "session-visit-first", foreignVisit, currentVisit, time.Now().UTC())
	if err == nil {
		t.Fatal("cross-task visit reassignment succeeded")
	}
	var count int
	if err := repo.db.Get(&count, `SELECT COUNT(*) FROM task_session_step_visits WHERE session_id = ? AND task_id = ?`, "session-visit-first", first.ID); err != nil {
		t.Fatalf("read association after rejection: %v", err)
	}
	if count != 1 {
		t.Fatalf("association count = %d, want original association retained", count)
	}
}

func TestReassignSessionStepVisitPreservesRetiredHistoryAndRejectsNewAssignment(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	ctx := context.Background()
	task := createStepTransitionsTestTask(t, repo, "task-visit-retired", "wf-visit-retired", "step-a")
	for _, stepID := range []string{"step-a", "step-b"} {
		if _, err := repo.db.Exec(`INSERT INTO workflow_steps (id, workflow_id, name) VALUES (?, ?, ?)`, stepID, task.WorkflowID, stepID); err != nil {
			t.Fatalf("create workflow step %s: %v", stepID, err)
		}
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session-visit-retired", TaskID: task.ID}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	var originalVisit string
	if err := repo.db.Get(&originalVisit, `SELECT visit_id FROM task_session_step_visits WHERE session_id = ?`, "session-visit-retired"); err != nil {
		t.Fatal(err)
	}
	task.WorkflowStepID = "step-b"
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("move task to step-b: %v", err)
	}
	var stepBVisit string
	if err := repo.db.Get(&stepBVisit, `SELECT visit_id FROM task_step_visits WHERE task_id = ? AND step_id = 'step-b'`, task.ID); err != nil {
		t.Fatal(err)
	}
	if err := repo.ReassignSessionStepVisit(ctx, task.ID, "session-visit-retired", stepBVisit, originalVisit, time.Now().UTC()); err != nil {
		t.Fatalf("reassign session: %v", err)
	}
	if err := repo.db.Get(&originalVisit, `SELECT visit_id FROM task_session_step_visits WHERE session_id = ?`, "session-visit-retired"); err != nil {
		t.Fatal(err)
	}
	if originalVisit != stepBVisit {
		t.Fatalf("reassigned visit = %q, want %q", originalVisit, stepBVisit)
	}
	if _, err := repo.db.Exec(`UPDATE workflow_steps SET retired_at = CURRENT_TIMESTAMP WHERE id = 'step-b'`); err != nil {
		t.Fatalf("retire step-b: %v", err)
	}
	visits, err := repo.ListTaskStepVisits(ctx, task.ID)
	if err != nil {
		t.Fatalf("list retired history: %v", err)
	}
	var retiredReadable bool
	for _, visit := range visits {
		if visit.VisitID == stepBVisit {
			retiredReadable = true
			if visit.Eligible {
				t.Fatal("retired visit remains eligible for new associations")
			}
			if len(visit.Sessions) != 1 || visit.Sessions[0].SessionID != "session-visit-retired" {
				t.Fatalf("retired visit sessions = %+v", visit.Sessions)
			}
		}
	}
	if !retiredReadable {
		t.Fatal("retired visit disappeared from history")
	}
	if err := repo.ReassignSessionStepVisit(ctx, task.ID, "session-visit-retired", stepBVisit, stepBVisit, time.Now().UTC()); err == nil {
		t.Fatal("reassignment to retired step succeeded")
	}
}
