package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
)

// newTaskSearchRepo opens a fresh file-backed SQLite repository.
func newTaskSearchRepo(t *testing.T) *Repository {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "task-search.db")
	conn, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlxDB := sqlx.NewDb(conn, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })
	repo, err := NewWithDB(sqlxDB, sqlxDB, nil)
	if err != nil {
		t.Fatalf("new repository: %v", err)
	}
	return repo
}

type taskSearchSeed struct {
	taskID string
}

// seedTaskSearch creates a workspace, workflow, task, and the requested
// sessions. It returns the task id.
func seedTaskSearch(t *testing.T, repo *Repository, sessionIDs ...string) taskSearchSeed {
	t.Helper()
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-search")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{
		ID: "wf-search", WorkspaceID: "ws-search", Name: "Search flow",
	}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-search", WorkspaceID: "ws-search", WorkflowID: "wf-search",
		Title: "Search task", State: "IN_PROGRESS",
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	for _, sessionID := range sessionIDs {
		if err := repo.CreateTaskSession(ctx, &models.TaskSession{
			ID: sessionID, TaskID: "task-search", State: models.TaskSessionStateIdle,
		}); err != nil {
			t.Fatalf("create session %s: %v", sessionID, err)
		}
	}
	return taskSearchSeed{taskID: "task-search"}
}

func seedSearchTurn(t *testing.T, repo *Repository, sessionID, turnID string, metadata map[string]interface{}) {
	t.Helper()
	if err := repo.CreateTurn(context.Background(), &models.Turn{
		ID: turnID, TaskSessionID: sessionID, TaskID: "task-search", Metadata: metadata,
	}); err != nil {
		t.Fatalf("create turn %s: %v", turnID, err)
	}
}

func seedMessage(t *testing.T, repo *Repository, id, sessionID, turnID, content string, createdAt time.Time) {
	t.Helper()
	seedMessageAs(t, repo, id, sessionID, turnID, content, createdAt, models.MessageAuthorUser)
}

// seedMessageAs inserts a message with an explicit author. Non-user messages
// are used where the per-session user-prompt monotonic-timestamp rule would
// otherwise reject a deliberate same-timestamp tie.
func seedMessageAs(
	t *testing.T, repo *Repository, id, sessionID, turnID, content string,
	createdAt time.Time, author models.MessageAuthorType,
) {
	t.Helper()
	if err := repo.CreateMessage(context.Background(), &models.Message{
		ID: id, TaskSessionID: sessionID, TaskID: "task-search", TurnID: turnID,
		AuthorType: author, Type: models.MessageTypeMessage,
		Content: content, CreatedAt: createdAt, UpdatedAt: createdAt,
	}); err != nil {
		t.Fatalf("create message %s: %v", id, err)
	}
}

func hitIDs(hits []*models.TaskMessageSearchHit) []string {
	ids := make([]string, 0, len(hits))
	for _, hit := range hits {
		ids = append(ids, hit.Message.ID)
	}
	return ids
}

func TestSearchTaskMessagesActiveFirstGlobalKeyset(t *testing.T) {
	repo := newTaskSearchRepo(t)
	seedTaskSearch(t, repo, "sess-active", "sess-other")
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	// Newest overall sits in the non-active session; the active bucket must
	// still come first, then recency.
	seedSearchTurn(t, repo, "sess-active", "turn-a", nil)
	seedSearchTurn(t, repo, "sess-other", "turn-b", nil)
	seedMessage(t, repo, "m-a1", "sess-active", "turn-a", "needle active one", base.Add(1*time.Second))
	seedMessage(t, repo, "m-a2", "sess-active", "turn-a", "needle active two", base.Add(2*time.Second))
	// Insert the non-active session oldest-first; user messages must keep a
	// strictly increasing timestamp per session.
	seedMessage(t, repo, "m-b2", "sess-other", "turn-b", "needle other two", base.Add(8*time.Second))
	seedMessage(t, repo, "m-b1", "sess-other", "turn-b", "needle other one", base.Add(9*time.Second))

	first, hasMore, err := repo.SearchTaskMessages(context.Background(), "task-search", models.SearchTaskMessagesOptions{
		Query: "needle", ActiveSessionID: "sess-active", Limit: 2,
	})
	if err != nil {
		t.Fatalf("search page 1: %v", err)
	}
	if !hasMore {
		t.Fatal("page 1 hasMore = false, want true")
	}
	if got := hitIDs(first); len(got) != 2 || got[0] != "m-a2" || got[1] != "m-a1" {
		t.Fatalf("page 1 ids = %v, want active-first [m-a2 m-a1]", got)
	}
	cursor := &models.SearchTaskMessagesCursor{
		Bucket: 0, Key: first[1].OrderKey, ID: first[1].Message.ID,
	}
	second, hasMore, err := repo.SearchTaskMessages(context.Background(), "task-search", models.SearchTaskMessagesOptions{
		Query: "needle", ActiveSessionID: "sess-active", Limit: 2, Cursor: cursor,
	})
	if err != nil {
		t.Fatalf("search page 2: %v", err)
	}
	if hasMore {
		t.Fatal("page 2 hasMore = true, want false")
	}
	// After the active bucket is exhausted the order continues in the
	// non-active bucket by recency; the active rows never recur.
	if got := hitIDs(second); len(got) != 2 || got[0] != "m-b1" || got[1] != "m-b2" {
		t.Fatalf("page 2 ids = %v, want [m-b1 m-b2]", got)
	}
}

func TestSearchTaskMessagesTurnStampAuthority(t *testing.T) {
	repo := newTaskSearchRepo(t)
	seedTaskSearch(t, repo, "sess-1")
	base := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	seedSearchTurn(t, repo, "sess-1", "turn-stamped", map[string]interface{}{
		models.TurnMetaKeyWorkflowStepIDAtStart: "step-plan",
	})
	seedSearchTurn(t, repo, "sess-1", "turn-legacy", nil)
	seedMessage(t, repo, "m-stamped", "sess-1", "turn-stamped", "stamp needle", base)
	seedMessage(t, repo, "m-legacy", "sess-1", "turn-legacy", "stamp needle", base.Add(time.Second))

	hits, _, err := repo.SearchTaskMessages(context.Background(), "task-search", models.SearchTaskMessagesOptions{
		Query: "stamp needle", Limit: 10,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	byID := map[string]*models.TaskMessageSearchHit{}
	for _, hit := range hits {
		byID[hit.Message.ID] = hit
	}
	if byID["m-stamped"] == nil || byID["m-stamped"].WorkflowStepID != "step-plan" {
		t.Fatalf("stamped hit step = %#v, want step-plan", byID["m-stamped"])
	}
	if byID["m-legacy"] == nil || byID["m-legacy"].WorkflowStepID != "" {
		t.Fatalf("legacy hit step = %#v, want empty", byID["m-legacy"])
	}
}

func TestSearchTaskMessagesIDTieBreak(t *testing.T) {
	repo := newTaskSearchRepo(t)
	seedTaskSearch(t, repo, "sess-1")
	at := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	seedSearchTurn(t, repo, "sess-1", "turn-1", nil)
	seedMessageAs(t, repo, "m-aaa", "sess-1", "turn-1", "tie needle", at, models.MessageAuthorAgent)
	seedMessageAs(t, repo, "m-bbb", "sess-1", "turn-1", "tie needle", at, models.MessageAuthorAgent)

	hits, _, err := repo.SearchTaskMessages(context.Background(), "task-search", models.SearchTaskMessagesOptions{
		Query: "tie needle", Limit: 10,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if got := hitIDs(hits); len(got) != 2 || got[0] != "m-bbb" || got[1] != "m-aaa" {
		t.Fatalf("tie-break ids = %v, want descending [m-bbb m-aaa]", got)
	}
}

func TestSearchTaskMessagesLiteralWildcards(t *testing.T) {
	repo := newTaskSearchRepo(t)
	seedTaskSearch(t, repo, "sess-1")
	at := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	seedSearchTurn(t, repo, "sess-1", "turn-1", nil)
	seedMessage(t, repo, "m-pct", "sess-1", "turn-1", "100% done", at)
	seedMessage(t, repo, "m-plain", "sess-1", "turn-1", "100 done", at.Add(time.Second))

	hits, _, err := repo.SearchTaskMessages(context.Background(), "task-search", models.SearchTaskMessagesOptions{
		Query: "100%", Limit: 10,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if got := hitIDs(hits); len(got) != 1 || got[0] != "m-pct" {
		t.Fatalf("wildcard ids = %v, want only [m-pct]", got)
	}
}
