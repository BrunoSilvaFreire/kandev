package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

// The upsert, the excerpt read, and the grouped reads are dialect-sensitive.
func TestPostgresTransitionFailureAndQuickChatBrowseQueries(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize postgres schema: %v", err)
	}
	if err := repo.migrateTaskTransitionFailures(); err != nil {
		t.Fatalf("replay transition failure schema: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Exec(db.Rebind(query), args...); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	exec(`INSERT INTO tasks (id, workspace_id, title, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		"task-pg", "ws-pg", "Chat", now, now)
	exec(`INSERT INTO tasks (id, workspace_id, parent_id, title, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"child-pg", "ws-pg", "task-pg", "Child", now, now)
	exec(`INSERT INTO task_sessions (id, task_id, state, started_at, updated_at) VALUES (?, ?, 'WAITING_FOR_INPUT', ?, ?)`,
		"session-pg", "task-pg", now, now)
	exec(`INSERT INTO task_session_turns (id, task_session_id, task_id, started_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"turn-pg", "session-pg", "task-pg", now, now, now)
	message := `INSERT INTO task_session_messages (id, task_session_id, task_id, turn_id, author_type, content, type, created_at, updated_at)
		VALUES (?, 'session-pg', 'task-pg', 'turn-pg', ?, ?, ?, ?, ?)`
	exec(message, "m1", "user", "first question", "message", now.Add(-2*time.Hour), now.Add(-2*time.Hour))
	exec(message, "m2", "agent", "answer", "message", now.Add(-time.Hour), now.Add(-time.Hour))
	exec(message, "m3", "agent", "Session resumed", "status", now, now)

	record := models.TaskTransitionFailure{
		TaskID: "task-pg", AttemptID: "a1", Source: models.TransitionFailureSourceDeferredMove,
		Failure: `{"code":"a"}`, CreatedAt: now,
	}
	if err := repo.SetTaskTransitionFailure(ctx, record); err != nil {
		t.Fatalf("set: %v", err)
	}
	record.AttemptID, record.Failure = "a2", `{"code":"b"}`
	if err := repo.SetTaskTransitionFailure(ctx, record); err != nil {
		t.Fatalf("supersede: %v", err)
	}
	got, err := repo.GetTaskTransitionFailure(ctx, "task-pg")
	if err != nil || got == nil || got.AttemptID != "a2" || !got.CreatedAt.Equal(now) {
		t.Fatalf("read = %+v, %v", got, err)
	}
	if cleared, err := repo.ClearTaskTransitionFailure(ctx, "task-pg"); err != nil || !cleared {
		t.Fatalf("clear = %v, %v", cleared, err)
	}

	first, err := repo.GetFirstUserMessageBySessionIDs(ctx, []string{"session-pg"})
	if err != nil || first["session-pg"] != "first question" {
		t.Fatalf("first user message = %+v, %v", first, err)
	}
	last, err := repo.GetLastConversationTimeBySessionIDs(ctx, []string{"session-pg"})
	if err != nil || !last["session-pg"].Equal(now.Add(-time.Hour)) {
		t.Fatalf("last conversation time = %+v, %v; the status row must not count", last, err)
	}
	children, err := repo.ListChildTaskRefsByParentIDs(ctx, []string{"task-pg"})
	if err != nil || len(children["task-pg"]) != 1 || children["task-pg"][0].ID != "child-pg" {
		t.Fatalf("children = %+v, %v", children, err)
	}
}
