package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	dbutil "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/steptelemetry"
	"github.com/kandev/kandev/internal/task/models"
)

func newRoutingProvenanceTestRepo(t *testing.T) *Repository {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "routing-provenance.db")
	dbConn, err := dbutil.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize schema: %v", err)
	}
	return repo
}

func seedTestTask(t *testing.T, repo *Repository, taskID string) {
	t.Helper()
	now := time.Now().UTC()
	task := &models.Task{
		ID:          taskID,
		WorkspaceID: "ws-1",
		Title:       "Test Task",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := repo.CreateTask(context.Background(), task); err != nil {
		t.Fatalf("create test task: %v", err)
	}
}

func TestRoutingProvenance_FreshSchema_CreatesTablesAndColumns(t *testing.T) {
	repo := newRoutingProvenanceTestRepo(t)
	ctx := context.Background()

	var tableName string
	if err := repo.db.GetContext(ctx, &tableName, `
		SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'task_session_routes'
	`); err != nil {
		t.Fatalf("task_session_routes table is missing: %v", err)
	}

	for _, col := range []string{"workflow_step_id_at_creation"} {
		var count int
		if err := repo.db.GetContext(ctx, &count, `
			SELECT COUNT(*) FROM pragma_table_info('task_sessions') WHERE name = ?
		`, col); err != nil || count == 0 {
			t.Fatalf("task_sessions missing column %s (count=%d, err=%v)", col, count, err)
		}
	}

	for _, col := range []string{"trigger_detail"} {
		var count int
		if err := repo.db.GetContext(ctx, &count, `
			SELECT COUNT(*) FROM pragma_table_info('task_step_transitions') WHERE name = ?
		`, col); err != nil || count == 0 {
			t.Fatalf("task_step_transitions missing column %s (count=%d, err=%v)", col, count, err)
		}
	}

	for _, col := range []string{"source_task_id", "source_session_id", "source_workflow_step_id"} {
		var count int
		if err := repo.db.GetContext(ctx, &count, `
			SELECT COUNT(*) FROM pragma_table_info('task_document_revisions') WHERE name = ?
		`, col); err != nil || count == 0 {
			t.Fatalf("task_document_revisions missing column %s (count=%d, err=%v)", col, count, err)
		}
	}
}

func TestRoutingProvenance_SessionCreationProvenance(t *testing.T) {
	repo := newRoutingProvenanceTestRepo(t)
	ctx := context.Background()
	taskID := "task-session-prov"
	seedTestTask(t, repo, taskID)

	now := time.Now().UTC()
	session := &models.TaskSession{
		ID:                       uuid.New().String(),
		TaskID:                   taskID,
		State:                    models.TaskSessionStateCreated,
		WorkflowStepIDAtCreation: "step-genesis",
		StartedAt:                now,
		UpdatedAt:                now,
	}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatalf("create task session: %v", err)
	}

	loaded, err := repo.GetTaskSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("get task session: %v", err)
	}
	if loaded.WorkflowStepIDAtCreation != "step-genesis" {
		t.Fatalf("WorkflowStepIDAtCreation = %q, want %q", loaded.WorkflowStepIDAtCreation, "step-genesis")
	}

	// Legacy row without workflow_step_id_at_creation
	legacyID := uuid.New().String()
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		INSERT INTO task_sessions (id, task_id, state, started_at, updated_at)
		VALUES (?, ?, 'CREATED', ?, ?)
	`), legacyID, taskID, now, now); err != nil {
		t.Fatalf("insert legacy session: %v", err)
	}

	loadedLegacy, err := repo.GetTaskSession(ctx, legacyID)
	if err != nil {
		t.Fatalf("get legacy session: %v", err)
	}
	if loadedLegacy.WorkflowStepIDAtCreation != "" {
		t.Fatalf("legacy WorkflowStepIDAtCreation = %q, want empty", loadedLegacy.WorkflowStepIDAtCreation)
	}
}

func TestRoutingProvenance_StepTransitionTriggerDetail(t *testing.T) {
	repo := newRoutingProvenanceTestRepo(t)
	ctx := context.Background()
	taskID := "task-transition-detail"
	seedTestTask(t, repo, taskID)

	detailJSON := `{"cause":"turn_completion","turn_id":"turn-1"}`
	ctxWithAttr := steptelemetry.WithAttribution(ctx, steptelemetry.Attribution{
		Trigger:       steptelemetry.TriggerEngineTransition,
		ActorKind:     steptelemetry.ActorSystem,
		TriggerDetail: detailJSON,
	})

	tx, err := repo.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	if _, err := repo.recordStepTransition(ctxWithAttr, tx, stepTransitionInput{
		taskID:             taskID,
		fromWorkflowStepID: "step-from",
		toWorkflowStepID:   "step-to",
		occurredAt:         time.Now().UTC(),
	}); err != nil {
		t.Fatalf("record step transition: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit tx: %v", err)
	}

	var storedDetail *string
	err = repo.db.GetContext(ctx, &storedDetail, repo.db.Rebind(`
		SELECT trigger_detail FROM task_step_transitions WHERE task_id = ? AND to_workflow_step_id = 'step-to'
	`), taskID)
	if err != nil {
		t.Fatalf("query trigger_detail: %v", err)
	}
	if storedDetail == nil || *storedDetail != detailJSON {
		t.Fatalf("stored trigger_detail = %v, want %q", storedDetail, detailJSON)
	}
}

func TestRoutingProvenance_DocumentRevisionProvenance(t *testing.T) {
	repo := newRoutingProvenanceTestRepo(t)
	ctx := context.Background()
	taskID := "task-doc-prov"
	seedTestTask(t, repo, taskID)

	now := time.Now().UTC()
	sessID := uuid.New().String()
	session := &models.TaskSession{
		ID:        sessID,
		TaskID:    taskID,
		State:     models.TaskSessionStateCreated,
		StartedAt: now,
		UpdatedAt: now,
	}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatalf("create session: %v", err)
	}

	doc := &models.TaskDocument{
		ID:        uuid.New().String(),
		TaskID:    taskID,
		Key:       "arch-notes",
		Title:     "Architecture Notes",
		Content:   "# Notes",
		CreatedAt: now,
		UpdatedAt: now,
	}
	stepID := "step-impl"
	rev := &models.TaskDocumentRevision{
		ID:                   uuid.New().String(),
		TaskID:               taskID,
		DocumentKey:          "arch-notes",
		Title:                "Architecture Notes",
		Content:              "# Notes",
		SourceTaskID:         &taskID,
		SourceSessionID:      &sessID,
		SourceWorkflowStepID: &stepID,
		CreatedAt:            now,
		UpdatedAt:            now,
	}

	if err := repo.WriteDocumentRevision(ctx, doc, rev, nil); err != nil {
		t.Fatalf("write document revision: %v", err)
	}

	loadedRev, err := repo.GetDocumentRevision(ctx, rev.ID)
	if err != nil {
		t.Fatalf("get document revision: %v", err)
	}
	if loadedRev.SourceTaskID == nil || *loadedRev.SourceTaskID != taskID {
		t.Fatalf("SourceTaskID = %v, want %s", loadedRev.SourceTaskID, taskID)
	}
	if loadedRev.SourceSessionID == nil || *loadedRev.SourceSessionID != sessID {
		t.Fatalf("SourceSessionID = %v, want %s", loadedRev.SourceSessionID, sessID)
	}
	if loadedRev.SourceWorkflowStepID == nil || *loadedRev.SourceWorkflowStepID != stepID {
		t.Fatalf("SourceWorkflowStepID = %v, want %s", loadedRev.SourceWorkflowStepID, stepID)
	}

	// Deleting the session must set source_session_id to NULL, not delete the document revision
	if err := repo.DeleteTaskSession(ctx, session); err != nil {
		t.Fatalf("delete task session: %v", err)
	}

	loadedAfterSessDelete, err := repo.GetDocumentRevision(ctx, rev.ID)
	if err != nil {
		t.Fatalf("get document revision after session delete: %v", err)
	}
	if loadedAfterSessDelete == nil {
		t.Fatal("document revision was unexpectedly deleted with session")
	}
	if loadedAfterSessDelete.SourceSessionID != nil {
		t.Fatalf("source_session_id after session delete = %v, want nil", loadedAfterSessDelete.SourceSessionID)
	}
}

// legacyTaskSessionsDDL is the pre-migration task_sessions shape: it still
// carries the deprecated bare workflow_step_id column and no
// workflow_step_id_at_creation provenance column.
const legacyTaskSessionsDDL = `
CREATE TABLE task_sessions (
	id TEXT PRIMARY KEY,
	task_id TEXT NOT NULL,
	agent_execution_id TEXT NOT NULL DEFAULT '',
	container_id TEXT NOT NULL DEFAULT '',
	agent_profile_id TEXT,
	execution_profile_id TEXT NOT NULL DEFAULT '',
	route_generation INTEGER NOT NULL DEFAULT 0,
	route_state TEXT NOT NULL DEFAULT '',
	route_reason TEXT NOT NULL DEFAULT '',
	downstream_acp_session_id TEXT NOT NULL DEFAULT '',
	executor_id TEXT DEFAULT '',
	executor_profile_id TEXT DEFAULT '',
	environment_id TEXT DEFAULT '',
	repository_id TEXT DEFAULT '',
	base_branch TEXT DEFAULT '',
	agent_profile_snapshot TEXT DEFAULT '{}',
	executor_snapshot TEXT DEFAULT '{}',
	environment_snapshot TEXT DEFAULT '{}',
	repository_snapshot TEXT DEFAULT '{}',
	state TEXT NOT NULL DEFAULT 'CREATED',
	error_message TEXT DEFAULT '',
	metadata TEXT DEFAULT '{}',
	started_at TIMESTAMP NOT NULL,
	completed_at TIMESTAMP,
	updated_at TIMESTAMP NOT NULL,
	is_primary INTEGER DEFAULT 0,
	is_passthrough INTEGER DEFAULT 0,
	review_status TEXT DEFAULT '',
	base_commit_sha TEXT DEFAULT '',
	task_environment_id TEXT DEFAULT '',
	workflow_step_id TEXT
)`

func taskSessionsColumnPresent(t *testing.T, repo *Repository, column string) bool {
	t.Helper()
	var count int
	if err := repo.db.GetContext(context.Background(), &count,
		`SELECT COUNT(*) FROM pragma_table_info('task_sessions') WHERE name = ?`, column); err != nil {
		t.Fatalf("pragma_table_info(%s): %v", column, err)
	}
	return count > 0
}

func taskSessionsDDL(t *testing.T, repo *Repository) string {
	t.Helper()
	var sqlText string
	if err := repo.db.GetContext(context.Background(), &sqlText,
		`SELECT sql FROM sqlite_master WHERE type='table' AND name='task_sessions'`); err != nil {
		t.Fatalf("load task_sessions DDL: %v", err)
	}
	return sqlText
}

// newLegacyTaskSessionsRepo builds a database whose task_sessions table is the
// pre-migration shape (bare workflow_step_id, no provenance) and then runs the
// real schema init + migrations over it, returning the upgraded repository and
// the id of the session row that must survive.
func newLegacyTaskSessionsRepo(t *testing.T) (*Repository, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "legacy-task-sessions.db")
	dbConn, err := dbutil.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys=OFF`); err != nil {
		t.Fatalf("disable foreign keys: %v", err)
	}
	if _, err := db.ExecContext(ctx, legacyTaskSessionsDDL); err != nil {
		t.Fatalf("create legacy task_sessions: %v", err)
	}
	now := time.Now().UTC()
	sessID := uuid.New().String()
	if _, err := db.ExecContext(ctx, db.Rebind(`
		INSERT INTO task_sessions (id, task_id, state, started_at, updated_at) VALUES (?, ?, 'CREATED', ?, ?)
	`), sessID, "task-legacy-upgrade", now, now); err != nil {
		t.Fatalf("insert legacy session: %v", err)
	}

	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize schema over legacy table: %v", err)
	}
	return repo, sessID
}

// TestRoutingProvenance_LegacySessionUpgrade proves a database whose
// task_sessions still carries the bare workflow_step_id column is recreated and
// then gains workflow_step_id_at_creation, with rows preserved.
func TestRoutingProvenance_LegacySessionUpgrade(t *testing.T) {
	repo, sessID := newLegacyTaskSessionsRepo(t)
	ctx := context.Background()

	if taskSessionsColumnPresent(t, repo, "workflow_step_id") {
		t.Fatal("upgrade left the deprecated workflow_step_id column in place")
	}
	if !taskSessionsColumnPresent(t, repo, "workflow_step_id_at_creation") {
		t.Fatal("upgrade did not add workflow_step_id_at_creation")
	}
	loaded, err := repo.GetTaskSession(ctx, sessID)
	if err != nil {
		t.Fatalf("session row did not survive the upgrade: %v", err)
	}
	if loaded.WorkflowStepIDAtCreation != "" {
		t.Fatalf("legacy session provenance = %q, want empty", loaded.WorkflowStepIDAtCreation)
	}
}

// TestRoutingProvenance_CurrentSchemaIsNotRecreated pins the fix that the
// recreate trigger is "workflow_step_id TEXT" rather than the bare column name:
// a current database (provenance column present, bare column absent) must not
// be recreated by a migration replay.
func TestRoutingProvenance_CurrentSchemaIsNotRecreated(t *testing.T) {
	repo := newRoutingProvenanceTestRepo(t)
	before := taskSessionsDDL(t, repo)

	if _, err := NewWithDB(repo.db, repo.db, nil); err != nil {
		t.Fatalf("replay migrations on current schema: %v", err)
	}

	if after := taskSessionsDDL(t, repo); after != before {
		t.Fatalf("current task_sessions was recreated by a replay\nbefore: %s\nafter: %s", before, after)
	}
	if !taskSessionsColumnPresent(t, repo, "workflow_step_id_at_creation") {
		t.Fatal("replay dropped workflow_step_id_at_creation")
	}
}

func TestRoutingProvenance_TaskSessionRoutes_CRUD_And_FKBehavior(t *testing.T) {
	repo := newRoutingProvenanceTestRepo(t)
	ctx := context.Background()
	taskID := "task-routes-test"
	seedTestTask(t, repo, taskID)

	now := time.Now().UTC()
	srcSessID := uuid.New().String()
	destSessID := uuid.New().String()
	for _, id := range []string{srcSessID, destSessID} {
		if err := repo.CreateTaskSession(ctx, &models.TaskSession{
			ID:        id,
			TaskID:    taskID,
			State:     models.TaskSessionStateCreated,
			StartedAt: now,
			UpdatedAt: now,
		}); err != nil {
			t.Fatalf("create session %s: %v", id, err)
		}
	}

	route := &models.TaskSessionRoute{
		ID:                        uuid.New().String(),
		TaskID:                    taskID,
		DestinationWorkflowStepID: "step-target",
		SourceSessionID:           &srcSessID,
		DestinationSessionID:      &destSessID,
		AgentProfileID:            "profile-eng",
		StartPolicy:               "reuse",
		EndPolicy:                 "park",
		Outcome:                   models.RoutingOutcomeReused,
		Reason:                    models.RoutingReasonReusedExisting,
		CorrelationID:             "corr-1",
		CreatedAt:                 now,
	}

	if err := repo.RecordSessionRoute(ctx, route); err != nil {
		t.Fatalf("record session route: %v", err)
	}

	routes, err := repo.ListTaskSessionRoutes(ctx, taskID)
	if err != nil {
		t.Fatalf("list task session routes: %v", err)
	}
	if len(routes) != 1 {
		t.Fatalf("got %d routes, want 1", len(routes))
	}
	r0 := routes[0]
	if r0.Reason != models.RoutingReasonReusedExisting {
		t.Fatalf("route reason = %q, want %q", r0.Reason, models.RoutingReasonReusedExisting)
	}
	if r0.Outcome != models.RoutingOutcomeReused {
		t.Fatalf("route outcome = %q, want %q", r0.Outcome, models.RoutingOutcomeReused)
	}

	// Idempotency check: recording the same route with same (task_id, correlation_id) is a no-op
	dupeRoute := *route
	dupeRoute.ID = uuid.New().String()
	if err := repo.RecordSessionRoute(ctx, &dupeRoute); err != nil {
		t.Fatalf("record duplicate session route: %v", err)
	}
	routesAfterDupe, err := repo.ListTaskSessionRoutes(ctx, taskID)
	if err != nil {
		t.Fatalf("list task session routes after duplicate: %v", err)
	}
	if len(routesAfterDupe) != 1 {
		t.Fatalf("got %d routes after duplicate, want 1", len(routesAfterDupe))
	}

	// Deleting source session sets source_session_id to NULL on route
	srcSession, err := repo.GetTaskSession(ctx, srcSessID)
	if err != nil {
		t.Fatalf("load source session: %v", err)
	}
	if err := repo.DeleteTaskSession(ctx, srcSession); err != nil {
		t.Fatalf("delete source session: %v", err)
	}
	routesAfterSessDelete, err := repo.ListTaskSessionRoutes(ctx, taskID)
	if err != nil {
		t.Fatalf("list routes after session delete: %v", err)
	}
	if len(routesAfterSessDelete) != 1 {
		t.Fatalf("route row missing after session delete")
	}
	if routesAfterSessDelete[0].SourceSessionID != nil {
		t.Fatalf("source_session_id = %v, want nil after session delete", routesAfterSessDelete[0].SourceSessionID)
	}
	if routesAfterSessDelete[0].DestinationSessionID == nil || *routesAfterSessDelete[0].DestinationSessionID != destSessID {
		t.Fatalf("destination_session_id = %v, want %s", routesAfterSessDelete[0].DestinationSessionID, destSessID)
	}
}

// TestSessionRouteDecisionDetailRoundTrip proves decision_detail is a nullable
// additive column: legacy rows read NULL and enriched rows survive verbatim.
func TestSessionRouteDecisionDetailRoundTrip(t *testing.T) {
	repo := newRoutingProvenanceTestRepo(t)
	ctx := context.Background()
	seedTestTask(t, repo, "task-detail")

	legacy := &models.TaskSessionRoute{
		ID: "route-legacy", TaskID: "task-detail", DestinationWorkflowStepID: "step-b",
		Outcome: models.RoutingOutcomeCreated, Reason: models.RoutingReasonNoReusableCandidate,
	}
	if err := repo.RecordSessionRoute(ctx, legacy); err != nil {
		t.Fatalf("record legacy route: %v", err)
	}
	detail := `{"start_policy":"new"}`
	enriched := &models.TaskSessionRoute{
		ID: "route-enriched", TaskID: "task-detail", DestinationWorkflowStepID: "step-b",
		Outcome: models.RoutingOutcomeCreated, Reason: models.RoutingReasonForcedNewPolicy,
		DecisionDetail: &detail,
	}
	if err := repo.RecordSessionRoute(ctx, enriched); err != nil {
		t.Fatalf("record enriched route: %v", err)
	}

	rows, err := repo.ListTaskSessionRoutes(ctx, "task-detail")
	if err != nil {
		t.Fatalf("list routes: %v", err)
	}
	byID := map[string]*models.TaskSessionRoute{}
	for _, row := range rows {
		byID[row.ID] = row
	}
	if byID["route-legacy"].DecisionDetail != nil {
		t.Fatalf("legacy route decision_detail = %v, want NULL", byID["route-legacy"].DecisionDetail)
	}
	if byID["route-enriched"].DecisionDetail == nil || *byID["route-enriched"].DecisionDetail != detail {
		t.Fatalf("enriched decision_detail = %v, want %q", byID["route-enriched"].DecisionDetail, detail)
	}
}
