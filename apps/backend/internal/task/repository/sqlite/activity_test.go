package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func insertTransition(
	t *testing.T, repo *Repository, taskID, from, to, sessionID, trigger string, occurredAt time.Time,
) {
	t.Helper()
	var session interface{}
	if sessionID != "" {
		session = sessionID
	}
	_, err := repo.db.ExecContext(context.Background(), repo.db.Rebind(`
		INSERT INTO task_step_transitions
			(task_id, session_id, from_workflow_id, from_workflow_step_id, to_workflow_id, to_workflow_step_id,
			 trigger, actor_kind, actor_id, trigger_detail, contract_version, occurred_at)
		VALUES (?, ?, 'wf', ?, 'wf', ?, ?, 'system', '', NULL, 1, ?)`),
		taskID, session, from, to, trigger, occurredAt)
	if err != nil {
		t.Fatalf("insert transition: %v", err)
	}
}

func TestListTaskActivityTransitionsOrdersAndFilters(t *testing.T) {
	repo := newTaskSearchRepo(t)
	seedTaskSearch(t, repo, "sess-1")
	if err := repo.CreateTask(context.Background(), &models.Task{
		ID: "task-other", WorkspaceID: "ws-search", WorkflowID: "wf-search", Title: "Other", State: "IN_PROGRESS",
	}); err != nil {
		t.Fatalf("create other task: %v", err)
	}
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	insertTransition(t, repo, "task-search", "", "step-a", "", "task_created", base)
	insertTransition(t, repo, "task-search", "step-a", "step-b", "", "turn_complete", base.Add(time.Second))
	insertTransition(t, repo, "task-search", "step-b", "step-a", "", "manual_move", base.Add(2*time.Second))
	insertTransition(t, repo, "task-other", "step-a", "step-b", "", "turn_complete", base.Add(3*time.Second))

	got, err := repo.ListTaskActivityTransitions(context.Background(), "task-search", models.TaskActivityFilter{Limit: 10})
	if err != nil {
		t.Fatalf("list transitions: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	if got[0].ToWorkflowStepID != "step-a" || got[1].ToWorkflowStepID != "step-b" {
		t.Fatalf("order = %s,%s, want step-a,step-b (newest first)", got[0].ToWorkflowStepID, got[1].ToWorkflowStepID)
	}

	cursor := base.Add(2 * time.Second)
	older, err := repo.ListTaskActivityTransitions(context.Background(), "task-search", models.TaskActivityFilter{
		Before: &cursor, Limit: 10,
	})
	if err != nil {
		t.Fatalf("list with cursor: %v", err)
	}
	if len(older) != 2 {
		t.Fatalf("older len = %d, want 2", len(older))
	}

	visits, err := repo.ListTaskActivityTransitions(context.Background(), "task-search", models.TaskActivityFilter{
		StepID: "step-b", Limit: 10,
	})
	if err != nil {
		t.Fatalf("list visits: %v", err)
	}
	if len(visits) != 1 || visits[0].FromWorkflowStepID != "step-a" {
		t.Fatalf("step-b visits = %#v, want the single a->b entry", visits)
	}
}

func TestCountCommittedTransitionPairsExcludesGenesis(t *testing.T) {
	repo := newTaskSearchRepo(t)
	seedTaskSearch(t, repo, "sess-1")
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	// Genesis (empty source) must not become an edge.
	insertTransition(t, repo, "task-search", "", "step-a", "", "task_created", base)
	insertTransition(t, repo, "task-search", "step-a", "step-b", "", "turn_complete", base.Add(time.Second))
	insertTransition(t, repo, "task-search", "step-a", "step-b", "", "manual_move", base.Add(2*time.Second))
	insertTransition(t, repo, "task-search", "step-b", "step-a", "", "manual_move", base.Add(3*time.Second))

	counts, err := repo.CountCommittedTransitionPairs(context.Background(), "task-search")
	if err != nil {
		t.Fatalf("count pairs: %v", err)
	}
	byPair := map[string]int{}
	for _, count := range counts {
		byPair[count.FromStepID+"->"+count.ToStepID] = count.Count
	}
	if byPair["step-a->step-b"] != 2 || byPair["step-b->step-a"] != 1 {
		t.Fatalf("counts = %#v, want a->b 2, b->a 1", byPair)
	}
	if _, ok := byPair["->step-a"]; ok {
		t.Fatal("genesis entry must not produce an edge count")
	}
}
