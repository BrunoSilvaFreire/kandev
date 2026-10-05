package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func seedActivityTask(t *testing.T, repo interface {
	CreateWorkspace(context.Context, *models.Workspace) error
	CreateWorkflow(context.Context, *models.Workflow) error
	CreateTask(context.Context, *models.Task) error
	CreateTaskSession(context.Context, *models.TaskSession) error
}) {
	t.Helper()
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-a", Name: "A"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-a", WorkspaceID: "ws-a", Name: "F"}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-a", WorkspaceID: "ws-a", WorkflowID: "wf-a", Title: "A", State: "IN_PROGRESS",
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "sess-a", TaskID: "task-a", State: models.TaskSessionStateIdle,
		Name: "Alpha", AgentProfileID: "profile-a", StartedAt: activityBase,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
}

var activityBase = time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

func TestListTaskActivityMergesAndPaginatesWithoutDuplicates(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedActivityTask(t, repo)
	ctx := context.Background()

	insert := func(from, to, trigger string, at time.Time) {
		_, err := repo.DB().ExecContext(ctx, `
			INSERT INTO task_step_transitions
				(task_id, session_id, from_workflow_id, from_workflow_step_id, to_workflow_id, to_workflow_step_id,
				 trigger, actor_kind, actor_id, trigger_detail, contract_version, occurred_at)
			VALUES ('task-a', 'sess-a', 'wf-a', ?, 'wf-a', ?, ?, 'system', '', NULL, 1, ?)`,
			from, to, trigger, at)
		if err != nil {
			t.Fatalf("insert transition: %v", err)
		}
	}
	insert("", "step-plan", "task_created", activityBase)
	insert("step-plan", "step-impl", "turn_complete", activityBase.Add(time.Second))
	insert("step-impl", "step-plan", "manual_move", activityBase.Add(2*time.Second))
	if err := repo.RecordSessionRoute(ctx, &models.TaskSessionRoute{
		TaskID: "task-a", DestinationWorkflowStepID: "step-impl",
		AgentProfileID: "profile-a", Outcome: models.RoutingOutcomeReused,
		Reason: models.RoutingReasonReusedExisting, CorrelationID: "corr-1",
		CreatedAt: activityBase.Add(90 * time.Minute),
	}); err != nil {
		t.Fatalf("record route: %v", err)
	}

	page1, hasMore, err := svc.ListTaskActivity(ctx, "task-a", models.TaskActivityFilter{Limit: 3})
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	if !hasMore {
		t.Fatal("page 1 hasMore = false, want true")
	}
	if len(page1) != 3 {
		t.Fatalf("page 1 len = %d, want 3", len(page1))
	}
	// Route is newest; the three transitions follow newest-first.
	if page1[0].Kind != models.ActivityKindRoute {
		t.Fatalf("page1[0] kind = %s, want route", page1[0].Kind)
	}
	for i := 1; i < len(page1); i++ {
		if !page1[i-1].OccurredAt.After(page1[i].OccurredAt) {
			t.Fatalf("page 1 not strictly newest-first at %d", i)
		}
	}

	cursor := page1[len(page1)-1].OccurredAt
	page2, _, err := svc.ListTaskActivity(ctx, "task-a", models.TaskActivityFilter{
		Limit: 3, Before: &cursor,
	})
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}
	seen := map[string]bool{}
	for _, event := range append(append([]*models.TaskActivityEvent{}, page1...), page2...) {
		key := string(event.Kind) + ":" + event.ID
		if seen[key] {
			t.Fatalf("duplicate event across pages: %s", key)
		}
		seen[key] = true
	}
}

func TestListTaskActivityStepFilterAndSessionlessVisit(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedActivityTask(t, repo)
	ctx := context.Background()
	// One manual transition authored by no session (legacy/unknown actor); it
	// must remain visible as a visit without a session.
	if _, err := repo.DB().ExecContext(ctx, `
		INSERT INTO task_step_transitions
			(task_id, session_id, from_workflow_id, from_workflow_step_id, to_workflow_id, to_workflow_step_id,
			 trigger, actor_kind, actor_id, trigger_detail, contract_version, occurred_at)
		VALUES ('task-a', NULL, 'wf-a', 'step-plan', 'wf-a', 'step-impl', 'manual_move', 'user', 'u1', NULL, 1, ?)`,
		activityBase.Add(time.Minute)); err != nil {
		t.Fatalf("insert: %v", err)
	}

	events, _, err := svc.ListTaskActivity(ctx, "task-a", models.TaskActivityFilter{
		StepID: "step-impl", Limit: 10,
	})
	if err != nil {
		t.Fatalf("filtered activity: %v", err)
	}
	found := false
	for _, event := range events {
		if event.Kind != models.ActivityKindTransition {
			t.Fatalf("step filter returned kind %s, want only transitions", event.Kind)
		}
		if event.Transition.SessionID == nil {
			found = true
		}
	}
	if !found {
		t.Fatal("expected a sessionless step visit to remain visible")
	}
}

func TestTaskTransitionCounts(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedActivityTask(t, repo)
	ctx := context.Background()
	for i, pair := range [][2]string{{"step-a", "step-b"}, {"step-a", "step-b"}, {"step-b", "step-a"}} {
		if _, err := repo.DB().ExecContext(ctx, `
			INSERT INTO task_step_transitions
				(task_id, session_id, from_workflow_id, from_workflow_step_id, to_workflow_id, to_workflow_step_id,
				 trigger, actor_kind, actor_id, trigger_detail, contract_version, occurred_at)
			VALUES ('task-a', NULL, 'wf-a', ?, 'wf-a', ?, 'turn_complete', 'system', '', NULL, 1, ?)`,
			pair[0], pair[1], activityBase.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	counts, err := svc.TaskTransitionSummary(ctx, "task-a")
	if err != nil {
		t.Fatalf("counts: %v", err)
	}
	got := map[string]int{}
	for _, count := range counts.Counts {
		got[fmt.Sprintf("%s->%s", count.FromStepID, count.ToStepID)] = count.Count
	}
	if got["step-a->step-b"] != 2 || got["step-b->step-a"] != 1 {
		t.Fatalf("counts = %#v", got)
	}
	visits := map[string]int{}
	for _, visit := range counts.Visits {
		visits[visit.StepID] = visit.Count
	}
	if visits["step-b"] != 2 || visits["step-a"] != 1 {
		t.Fatalf("visits = %#v, want step-b 2, step-a 1", visits)
	}
}

func TestListTaskActivityStepScopeAttachesRoute(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedActivityTask(t, repo)
	ctx := context.Background()

	result, err := repo.DB().ExecContext(ctx, `
		INSERT INTO task_step_transitions
			(task_id, session_id, from_workflow_id, from_workflow_step_id, to_workflow_id, to_workflow_step_id,
			 trigger, actor_kind, actor_id, trigger_detail, contract_version, occurred_at)
		VALUES ('task-a', NULL, 'wf-a', 'step-plan', 'wf-a', 'step-impl', 'turn_complete', 'system', '', NULL, 1, ?)`,
		activityBase.Add(time.Minute))
	if err != nil {
		t.Fatalf("insert transition: %v", err)
	}
	transitionID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("last insert id: %v", err)
	}
	destination := "sess-dest"
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "sess-dest", TaskID: "task-a", State: models.TaskSessionStateIdle,
		StartedAt: activityBase.Add(time.Minute),
	}); err != nil {
		t.Fatalf("create destination session: %v", err)
	}
	if err := repo.RecordSessionRoute(ctx, &models.TaskSessionRoute{
		TaskID:                    "task-a",
		DestinationWorkflowStepID: "step-impl",
		DestinationSessionID:      &destination,
		AgentProfileID:            "profile-a",
		Outcome:                   models.RoutingOutcomeCreated,
		Reason:                    models.RoutingReasonNoReusableCandidate,
		WorkflowStepTransitionID:  &transitionID,
		CorrelationID:             "corr-step-route",
		CreatedAt:                 activityBase.Add(time.Minute),
	}); err != nil {
		t.Fatalf("record route: %v", err)
	}

	events, _, err := svc.ListTaskActivity(ctx, "task-a", models.TaskActivityFilter{
		StepID: "step-impl", Limit: 10,
	})
	if err != nil {
		t.Fatalf("step activity: %v", err)
	}
	var visit *models.TaskActivityEvent
	for _, event := range events {
		if event.Kind == models.ActivityKindTransition {
			visit = event
		}
	}
	if visit == nil {
		t.Fatal("expected the step-impl visit transition")
	}
	if visit.Route == nil || visit.Route.DestinationSessionID == nil ||
		*visit.Route.DestinationSessionID != "sess-dest" {
		t.Fatalf("visit route = %#v, want attached route with destination sess-dest", visit.Route)
	}
	if visit.Route.Outcome != models.RoutingOutcomeCreated ||
		visit.Route.Reason != models.RoutingReasonNoReusableCandidate {
		t.Fatalf("visit route decision = %s/%s", visit.Route.Outcome, visit.Route.Reason)
	}
}

func TestListTaskActivityReviewRunsPaginateWithoutSkip(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedActivityTask(t, repo)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		if err := repo.CreateTaskReviewRun(ctx, &models.TaskReviewRun{
			ID:        fmt.Sprintf("run-%d", i),
			TaskID:    "task-a",
			Status:    models.ReviewRunCompleted,
			CreatedAt: activityBase.Add(time.Duration(i+1) * time.Minute),
		}); err != nil {
			t.Fatalf("create review run %d: %v", i, err)
		}
	}

	seen := map[string]bool{}
	var cursor *time.Time
	for page := 0; page < 20; page++ {
		events, hasMore, err := svc.ListTaskActivity(ctx, "task-a", models.TaskActivityFilter{
			Limit: 2, Before: cursor,
		})
		if err != nil {
			t.Fatalf("page %d: %v", page, err)
		}
		for _, event := range events {
			if event.Kind == models.ActivityKindReviewRun {
				seen[event.ID] = true
			}
		}
		if !hasMore || len(events) == 0 {
			break
		}
		last := events[len(events)-1].OccurredAt
		cursor = &last
	}
	if len(seen) != 5 {
		t.Fatalf("review runs seen across pages = %d, want 5 (cursor must not skip older runs)", len(seen))
	}
}

func TestListTaskActivityReviewRunsRespectStepFilter(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedActivityTask(t, repo)
	ctx := context.Background()
	runs := []struct {
		id     string
		stepID string
	}{
		{id: "run-impl", stepID: "step-impl"},
		{id: "run-review", stepID: "step-review"},
	}
	for i, run := range runs {
		if err := repo.CreateTaskReviewRun(ctx, &models.TaskReviewRun{
			ID:             run.id,
			TaskID:         "task-a",
			WorkflowStepID: run.stepID,
			Status:         models.ReviewRunCompleted,
			CreatedAt:      activityBase.Add(time.Duration(i+1) * time.Minute),
		}); err != nil {
			t.Fatalf("create review run %s: %v", run.id, err)
		}
	}

	events, _, err := svc.ListTaskActivity(ctx, "task-a", models.TaskActivityFilter{
		StepID: "step-impl", Limit: 10,
	})
	if err != nil {
		t.Fatalf("step activity: %v", err)
	}
	seen := map[string]bool{}
	for _, event := range events {
		if event.Kind == models.ActivityKindReviewRun {
			seen[event.ID] = true
		}
	}
	if !seen["run-impl"] || seen["run-review"] {
		t.Fatalf("step-filtered review runs = %#v, want only run-impl", seen)
	}
}
