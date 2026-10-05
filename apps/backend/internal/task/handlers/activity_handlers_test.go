package handlers

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/dto"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func newActivityTestHandlers(t *testing.T) (*TaskHandlers, *taskrepo.Repository) {
	t.Helper()
	h, repo := newPlanTestHandlersWithRepo(t)
	h.service = service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo, Workflows: repo,
		Messages: repo, Turns: repo, Sessions: repo, GitSnapshots: repo,
		RepoEntities: repo, Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, h.logger, service.RepositoryDiscoveryConfig{})
	return h, repo
}

func insertActivityTransition(t *testing.T, repo *taskrepo.Repository, taskID, from, to string, at time.Time) {
	t.Helper()
	_, err := repo.DB().ExecContext(context.Background(), `
		INSERT INTO task_step_transitions
			(task_id, session_id, from_workflow_id, from_workflow_step_id, to_workflow_id, to_workflow_step_id,
			 trigger, actor_kind, actor_id, trigger_detail, contract_version, occurred_at)
		VALUES (?, NULL, 'wf', ?, 'wf', ?, 'turn_complete', 'system', '', NULL, 1, ?)`,
		taskID, from, to, at)
	if err != nil {
		t.Fatalf("insert transition: %v", err)
	}
}

func TestWSListTaskActivityValidatesAndPages(t *testing.T) {
	h, repo := newActivityTestHandlers(t)
	ctx := context.Background()
	base := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	insertActivityTransition(t, repo, planTaskID, "step-a", "step-b", base)
	insertActivityTransition(t, repo, planTaskID, "step-b", "step-a", base.Add(time.Second))

	missing, err := h.wsListTaskActivity(ctx, planMsg(t, ws.ActionTaskActivityList, `{"limit":5}`))
	if err != nil {
		t.Fatalf("missing task: %v", err)
	}
	if code := wsWorkflowError(t, missing).Message; code != "task_id is required" {
		t.Fatalf("missing task message = %q", code)
	}

	out, err := h.wsListTaskActivity(ctx, planMsg(t, ws.ActionTaskActivityList,
		`{"task_id":"`+planTaskID+`","limit":1}`))
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	var page dto.TaskActivityListResponse
	if err := json.Unmarshal(out.Payload, &page); err != nil {
		t.Fatalf("unmarshal page: %v", err)
	}
	if len(page.Events) != 1 || !page.HasMore || page.NextCursor == "" {
		t.Fatalf("page 1 = %#v, want 1 event, has_more, cursor", page)
	}
	if page.Events[0].Transition == nil || page.Events[0].Transition.ToStepID != "step-a" {
		t.Fatalf("page 1 newest event = %#v, want step-a destination", page.Events[0])
	}

	page2Out, err := h.wsListTaskActivity(ctx, planMsg(t, ws.ActionTaskActivityList,
		`{"task_id":"`+planTaskID+`","limit":1,"cursor":"`+page.NextCursor+`"}`))
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}
	var page2 dto.TaskActivityListResponse
	if err := json.Unmarshal(page2Out.Payload, &page2); err != nil {
		t.Fatalf("unmarshal page 2: %v", err)
	}
	if len(page2.Events) != 1 || page2.Events[0].Transition.ToStepID != "step-b" {
		t.Fatalf("page 2 = %#v, want the older step-b event", page2.Events)
	}

	bad, err := h.wsListTaskActivity(ctx, planMsg(t, ws.ActionTaskActivityList,
		`{"task_id":"`+planTaskID+`","cursor":"!!!"}`))
	if err != nil {
		t.Fatalf("bad cursor: %v", err)
	}
	if code := wsWorkflowError(t, bad).Message; code != "Invalid activity cursor" {
		t.Fatalf("bad cursor message = %q", code)
	}
}

func TestWSTaskTransitionCounts(t *testing.T) {
	h, repo := newActivityTestHandlers(t)
	ctx := context.Background()
	base := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	insertActivityTransition(t, repo, planTaskID, "step-a", "step-b", base)
	insertActivityTransition(t, repo, planTaskID, "step-a", "step-b", base.Add(time.Second))
	insertActivityTransition(t, repo, planTaskID, "step-b", "step-a", base.Add(2*time.Second))

	out, err := h.wsTaskTransitionCounts(ctx, planMsg(t, ws.ActionTaskTransitionCounts,
		`{"task_id":"`+planTaskID+`"}`))
	if err != nil {
		t.Fatalf("counts: %v", err)
	}
	var body dto.TaskTransitionCountsResponse
	if err := json.Unmarshal(out.Payload, &body); err != nil {
		t.Fatalf("unmarshal counts: %v", err)
	}
	got := map[string]int{}
	for _, count := range body.Counts {
		got[count.FromStepID+"->"+count.ToStepID] = count.Count
	}
	if got["step-a->step-b"] != 2 || got["step-b->step-a"] != 1 {
		t.Fatalf("counts = %#v", got)
	}
}
