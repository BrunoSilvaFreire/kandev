package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// A confirmed recovery resolves only its own session's episode. The task
// warning survives an unresolved sibling, and a stale episode ID clears nothing.
func TestResolveInterruptionAfterRecovery(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session2", TaskID: "task1", State: models.TaskSessionStateWaitingForInput,
		StartedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create sibling session: %v", err)
	}
	for _, id := range []string{"session1", "session2"} {
		if err := repo.MarkSessionInterrupted(ctx, id, models.InterruptionReasonBackendRestart, "", now); err != nil {
			t.Fatalf("mark %s interrupted: %v", id, err)
		}
	}
	const marker = "2026-10-10T00:00:00Z"
	if _, err := repo.SetTaskMetadataKeyIfNotArchived(ctx, "task1", models.MetaKeyInterruptedAt, marker); err != nil {
		t.Fatalf("set marker: %v", err)
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})

	episode := func(id string) string {
		t.Helper()
		session, err := repo.GetTaskSession(ctx, id)
		if err != nil {
			t.Fatalf("load %s: %v", id, err)
		}
		return session.InterruptionEpisodeID
	}
	marked := func() bool {
		t.Helper()
		task, err := repo.GetTask(ctx, "task1")
		if err != nil {
			t.Fatalf("load task: %v", err)
		}
		return task.Metadata[models.MetaKeyInterruptedAt] != nil
	}
	resolve := func(id, episodeID string) {
		svc.resolveInterruptionAfterRecovery(ctx, &models.TaskSession{ID: id, TaskID: "task1"},
			interruptedMarkerSnapshot{value: marker, captured: true, episodeID: episodeID})
	}

	first, second := episode("session1"), episode("session2")

	resolve("session1", "stale-episode")
	if episode("session1") != first || !marked() {
		t.Fatal("a stale episode ID must not resolve the episode or clear the warning")
	}

	resolve("session1", first)
	if episode("session1") != "" {
		t.Fatal("expected session1 episode to be resolved")
	}
	if episode("session2") != second || !marked() {
		t.Fatal("an unresolved sibling must keep its episode and the task warning")
	}
	task, _ := repo.GetTask(ctx, "task1")
	if task.Interruption == nil || task.Interruption.SessionID != "session2" {
		t.Fatalf("expected projection to move to session2, got %+v", task.Interruption)
	}

	resolve("session2", second)
	if marked() {
		t.Fatal("expected the warning to clear once no episode remains")
	}
	if task, _ := repo.GetTask(ctx, "task1"); task.Interruption != nil {
		t.Fatalf("expected no projected interruption, got %+v", task.Interruption)
	}
}
