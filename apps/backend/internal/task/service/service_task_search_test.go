package service

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// seedTaskSearchService builds a task with two sessions and three messages
// (two in the active session, one in another) directly through the repository.
func seedTaskSearchService(t *testing.T, repo interface {
	CreateWorkspace(context.Context, *models.Workspace) error
	CreateWorkflow(context.Context, *models.Workflow) error
	CreateTask(context.Context, *models.Task) error
	CreateTaskSession(context.Context, *models.TaskSession) error
	CreateTurn(context.Context, *models.Turn) error
	CreateMessage(context.Context, *models.Message) error
}) {
	t.Helper()
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-s", Name: "S"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-s", WorkspaceID: "ws-s", Name: "F"}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-s", WorkspaceID: "ws-s", WorkflowID: "wf-s", Title: "T", State: "IN_PROGRESS",
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "sess-a", TaskID: "task-s", State: models.TaskSessionStateIdle,
		Name: "Session Alpha", AgentProfileID: "profile-1",
	}); err != nil {
		t.Fatalf("create session a: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "sess-b", TaskID: "task-s", State: models.TaskSessionStateIdle,
	}); err != nil {
		t.Fatalf("create session b: %v", err)
	}
	if err := repo.CreateTurn(ctx, &models.Turn{
		ID: "turn-a", TaskSessionID: "sess-a", TaskID: "task-s",
		Metadata: map[string]interface{}{models.TurnMetaKeyWorkflowStepIDAtStart: "step-plan"},
	}); err != nil {
		t.Fatalf("create turn a: %v", err)
	}
	if err := repo.CreateTurn(ctx, &models.Turn{
		ID: "turn-b", TaskSessionID: "sess-b", TaskID: "task-s",
	}); err != nil {
		t.Fatalf("create turn b: %v", err)
	}
	base := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	create := func(id, session, turn string, at time.Time) {
		if err := repo.CreateMessage(ctx, &models.Message{
			ID: id, TaskSessionID: session, TaskID: "task-s", TurnID: turn,
			AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeMessage,
			Content: "service needle", CreatedAt: at, UpdatedAt: at,
		}); err != nil {
			t.Fatalf("create message %s: %v", id, err)
		}
	}
	create("m-a1", "sess-a", "turn-a", base)
	create("m-a2", "sess-a", "turn-a", base.Add(time.Second))
	create("m-b1", "sess-b", "turn-b", base.Add(2*time.Second))
}

func TestServiceSearchTaskMessagesEnrichesAndPages(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedTaskSearchService(t, repo)
	ctx := context.Background()

	page1, hasMore, err := svc.SearchTaskMessages(ctx, "task-s", "sess-a", "service needle", 2, nil)
	if err != nil {
		t.Fatalf("search page 1: %v", err)
	}
	if !hasMore || len(page1) != 2 {
		t.Fatalf("page 1 len=%d hasMore=%v, want 2/true", len(page1), hasMore)
	}
	if page1[0].Message.ID != "m-a2" || page1[1].Message.ID != "m-a1" {
		t.Fatalf("page 1 order = %s,%s, want m-a2,m-a1", page1[0].Message.ID, page1[1].Message.ID)
	}
	if page1[0].Session == nil || page1[0].Session.Name != "Session Alpha" ||
		page1[0].Session.AgentProfileID != "profile-1" {
		t.Fatalf("page 1 session enrichment = %#v", page1[0].Session)
	}
	if page1[0].WorkflowStepID != "step-plan" {
		t.Fatalf("page 1 step = %q, want step-plan", page1[0].WorkflowStepID)
	}

	cursor := &models.SearchTaskMessagesCursor{Bucket: 0, Key: page1[1].OrderKey, ID: page1[1].Message.ID}
	page2, hasMore, err := svc.SearchTaskMessages(ctx, "task-s", "sess-a", "service needle", 2, cursor)
	if err != nil {
		t.Fatalf("search page 2: %v", err)
	}
	if hasMore || len(page2) != 1 || page2[0].Message.ID != "m-b1" {
		t.Fatalf("page 2 = %#v hasMore=%v, want [m-b1]/false", page2, hasMore)
	}
	if page2[0].WorkflowStepID != "" {
		t.Fatalf("page 2 legacy step = %q, want empty", page2[0].WorkflowStepID)
	}
}
