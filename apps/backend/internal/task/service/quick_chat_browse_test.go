package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func seedQuickChatMessage(t *testing.T, sqlxDB *sqlx.DB, taskID, id, author, content string, at time.Time) {
	t.Helper()
	ctx := context.Background()
	sessionID := taskID + "-session"
	if _, err := sqlxDB.ExecContext(ctx, `
		INSERT INTO task_session_turns (id, task_session_id, task_id, started_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT (id) DO NOTHING`,
		sessionID+"-turn", sessionID, taskID, at, at, at); err != nil {
		t.Fatalf("insert turn: %v", err)
	}
	if _, err := sqlxDB.ExecContext(ctx, `
		INSERT INTO task_session_messages (id, task_session_id, task_id, turn_id, author_type, content, type, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, 'message', ?, ?)`,
		id, sessionID, taskID, sessionID+"-turn", author, content, at, at); err != nil {
		t.Fatalf("insert message %s: %v", id, err)
	}
}

// The browse detail is conversation evidence: the newest message dates the
// row whatever happened to the session record, a name is never replaced, and
// links come only from the chat's own repositories and the tasks it created.
func TestQuickChatBrowseDetailsUseConversationEvidence(t *testing.T) {
	svc, sqlxDB := createOfficeIntegrationServiceWithDB(t)
	ctx := context.Background()
	if err := svc.workspaces.CreateWorkspace(ctx, &models.Workspace{ID: "ws-qc", Name: "ws"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	// Every session row was rewritten at the same instant, as after a restart.
	restart := base.Add(48 * time.Hour)
	seedQuickChatTask(t, svc, sqlxDB, quickChatFixture{id: "task-named", title: "Release notes", updatedAt: restart})
	seedQuickChatTask(t, svc, sqlxDB, quickChatFixture{id: "task-untitled", title: quickChatDefaultTitle, updatedAt: restart})
	seedQuickChatTask(t, svc, sqlxDB, quickChatFixture{id: "task-empty", title: quickChatDefaultTitle, updatedAt: restart})

	seedQuickChatMessage(t, sqlxDB, "task-named", "m1", "user", "draft the notes", base.Add(time.Hour))
	seedQuickChatMessage(t, sqlxDB, "task-named", "m2", "agent", "done", base.Add(2*time.Hour))
	long := "  Why does\nthe   importer " + strings.Repeat("é", 100)
	seedQuickChatMessage(t, sqlxDB, "task-untitled", "m3", "agent", "hello, how can I help?", base.Add(3*time.Hour))
	seedQuickChatMessage(t, sqlxDB, "task-untitled", "m4", "user", long, base.Add(4*time.Hour))
	seedQuickChatMessage(t, sqlxDB, "task-untitled", "m5", "user", "second question", base.Add(5*time.Hour))
	// Lifecycle rows written at the restart are not conversation.
	for _, taskID := range []string{"task-named", "task-untitled", "task-empty"} {
		if _, err := sqlxDB.ExecContext(ctx, `
			INSERT INTO task_session_turns (id, task_session_id, task_id, started_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT (id) DO NOTHING`,
			taskID+"-session-turn", taskID+"-session", taskID, restart, restart, restart); err != nil {
			t.Fatalf("insert turn: %v", err)
		}
		if _, err := sqlxDB.ExecContext(ctx, `
			INSERT INTO task_session_messages (id, task_session_id, task_id, turn_id, author_type, content, type, created_at, updated_at)
			VALUES (?, ?, ?, ?, 'agent', 'Session resumed', 'status', ?, ?)`,
			taskID+"-status", taskID+"-session", taskID, taskID+"-session-turn", restart, restart); err != nil {
			t.Fatalf("insert status row: %v", err)
		}
	}

	// A repository attached to one chat, and tasks one chat created.
	if err := svc.repoEntities.CreateRepository(ctx, &models.Repository{
		ID: "repo-1", WorkspaceID: "ws-qc", Name: "repo-1", SourceType: "local", LocalPath: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}
	if _, err := sqlxDB.ExecContext(ctx, `
		INSERT INTO task_repositories (id, task_id, repository_id, base_branch, position, created_at, updated_at)
		VALUES ('tr-1', 'task-named', 'repo-1', 'main', 0, ?, ?)`, base, base); err != nil {
		t.Fatalf("insert task repository: %v", err)
	}
	for index := 0; index < 7; index++ {
		id := "child-" + string(rune('a'+index))
		if err := svc.tasks.CreateTask(ctx, &models.Task{
			ID: id, WorkspaceID: "ws-qc", ParentID: "task-untitled", Title: "Child " + id,
			State: v1.TaskStateTODO, Priority: "medium",
		}); err != nil {
			t.Fatalf("CreateTask(%s): %v", id, err)
		}
	}
	if _, err := sqlxDB.ExecContext(ctx, `UPDATE tasks SET archived_at = ? WHERE id = 'child-a'`, base); err != nil {
		t.Fatalf("archive child: %v", err)
	}

	items, err := svc.ListQuickChatSessions(ctx, "ws-qc")
	if err != nil {
		t.Fatalf("ListQuickChatSessions: %v", err)
	}
	details, err := svc.QuickChatBrowseDetails(ctx, items)
	if err != nil {
		t.Fatalf("QuickChatBrowseDetails: %v", err)
	}

	named := details["task-named-session"]
	if !named.LastActivityAt.Equal(base.Add(2*time.Hour)) || named.ActivitySource != QuickChatActivitySourceMessage {
		t.Fatalf("named activity = %v/%s, want the newest message", named.LastActivityAt, named.ActivitySource)
	}
	if named.TitleFallback != "" {
		t.Fatalf("a named chat must not get a fallback title, got %q", named.TitleFallback)
	}
	if len(named.RepositoryIDs) != 1 || named.RepositoryIDs[0] != "repo-1" || named.TaskCount != 0 {
		t.Fatalf("named links = %+v", named)
	}

	untitled := details["task-untitled-session"]
	if !untitled.LastActivityAt.Equal(base.Add(5 * time.Hour)) {
		t.Fatalf("untitled activity = %v", untitled.LastActivityAt)
	}
	wantPrefix := "Why does the importer "
	if !strings.HasPrefix(untitled.TitleFallback, wantPrefix) || !strings.HasSuffix(untitled.TitleFallback, "…") ||
		len([]rune(untitled.TitleFallback)) != quickChatTitleFallbackRunes+1 {
		t.Fatalf("fallback title = %q", untitled.TitleFallback)
	}
	if len(untitled.RepositoryIDs) != 0 {
		t.Fatalf("a chat with no attached repository must have no repository link: %+v", untitled.RepositoryIDs)
	}
	if untitled.TaskCount != 6 || len(untitled.Tasks) != quickChatBrowseTaskLimit || untitled.Tasks[0].ID != "child-b" {
		t.Fatalf("created tasks = %d %+v, want 6 unarchived with the first five listed", untitled.TaskCount, untitled.Tasks)
	}

	empty := details["task-empty-session"]
	if empty.ActivitySource != QuickChatActivitySourceCreated || empty.TitleFallback != "" || empty.LastActivityAt.Equal(restart) {
		t.Fatalf("empty chat = %+v, want its creation time and no title", empty)
	}
}
