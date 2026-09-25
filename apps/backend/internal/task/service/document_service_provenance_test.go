package service

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/sqlite"
)

// seedDocumentSession inserts a session row so revision provenance can satisfy
// the source_session_id foreign key.
func seedDocumentSession(t *testing.T, repo *sqlite.Repository, taskID, sessionID string) {
	t.Helper()
	now := time.Now().UTC()
	if err := repo.CreateTaskSession(context.Background(), &models.TaskSession{
		ID: sessionID, TaskID: taskID, AgentProfileID: "profile-1",
		State: models.TaskSessionStateRunning, StartedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create session %s: %v", sessionID, err)
	}
}

func latestDocumentRevisionForTest(t *testing.T, svc *DocumentService, taskID, key string) *struct {
	sourceTask    string
	sourceSession string
	sourceStep    string
	revision      int
} {
	t.Helper()
	revisions, err := svc.ListRevisions(context.Background(), taskID, key, 1)
	if err != nil {
		t.Fatalf("list revisions: %v", err)
	}
	if len(revisions) == 0 {
		t.Fatalf("no revisions for %s/%s", taskID, key)
	}
	rev := revisions[0]
	out := &struct {
		sourceTask    string
		sourceSession string
		sourceStep    string
		revision      int
	}{revision: rev.RevisionNumber}
	if rev.SourceTaskID != nil {
		out.sourceTask = *rev.SourceTaskID
	}
	if rev.SourceSessionID != nil {
		out.sourceSession = *rev.SourceSessionID
	}
	if rev.SourceWorkflowStepID != nil {
		out.sourceStep = *rev.SourceWorkflowStepID
	}
	return out
}

func TestDocumentServicePersistsRevisionProvenance(t *testing.T) {
	svc, repo := newDocumentTestService(t)
	seedDocumentSession(t, repo, "task-doc", "session-1")
	ctx := context.Background()

	_, err := svc.CreateOrUpdateDocumentWithProvenance(ctx, "task-doc", "spec", "custom", "Spec", "body",
		"agent", "Agent", DocumentWriteProvenance{
			SourceTaskID:         "task-doc",
			SourceSessionID:      "session-1",
			SourceWorkflowStepID: "step-1",
		})
	if err != nil {
		t.Fatalf("CreateOrUpdateDocumentWithProvenance: %v", err)
	}

	latest := latestDocumentRevisionForTest(t, svc, "task-doc", "spec")
	if latest.sourceTask != "task-doc" || latest.sourceSession != "session-1" || latest.sourceStep != "step-1" {
		t.Fatalf("provenance = %+v, want task-doc/session-1/step-1", latest)
	}
}

func TestDocumentServiceNilProvenanceStaysLegacy(t *testing.T) {
	svc, _ := newDocumentTestService(t)
	ctx := context.Background()

	if _, err := svc.CreateOrUpdateDocument(ctx, "task-doc", "spec", "custom", "Spec", "body", "user", "Ada"); err != nil {
		t.Fatalf("CreateOrUpdateDocument: %v", err)
	}

	latest := latestDocumentRevisionForTest(t, svc, "task-doc", "spec")
	if latest.sourceTask != "" || latest.sourceSession != "" || latest.sourceStep != "" {
		t.Fatalf("legacy provenance = %+v, want all empty", latest)
	}
}

func TestDocumentServiceDoesNotCoalesceAcrossDifferentSources(t *testing.T) {
	svc, repo := newDocumentTestService(t)
	seedDocumentSession(t, repo, "task-doc", "session-1")
	seedDocumentSession(t, repo, "task-doc", "session-2")
	ctx := context.Background()
	provenanceA := DocumentWriteProvenance{SourceTaskID: "task-doc", SourceSessionID: "session-1", SourceWorkflowStepID: "step-1"}
	provenanceB := DocumentWriteProvenance{SourceTaskID: "task-doc", SourceSessionID: "session-2", SourceWorkflowStepID: "step-1"}

	if _, err := svc.CreateOrUpdateDocumentWithProvenance(ctx, "task-doc", "spec", "custom", "Spec", "v1", "agent", "Agent", provenanceA); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if _, err := svc.CreateOrUpdateDocumentWithProvenance(ctx, "task-doc", "spec", "custom", "Spec", "v2", "agent", "Agent", provenanceB); err != nil {
		t.Fatalf("second write: %v", err)
	}

	revisions, err := svc.ListRevisions(ctx, "task-doc", "spec", 0)
	if err != nil {
		t.Fatalf("list revisions: %v", err)
	}
	if len(revisions) != 2 {
		t.Fatalf("revision count = %d, want 2 (a different source must append its own revision)", len(revisions))
	}
	// Newest first: session-2 wrote the second revision; session-1 owns the first.
	if revisions[0].SourceSessionID == nil || *revisions[0].SourceSessionID != "session-2" {
		t.Fatalf("newest provenance = %v, want session-2", revisions[0].SourceSessionID)
	}
	if revisions[1].SourceSessionID == nil || *revisions[1].SourceSessionID != "session-1" {
		t.Fatalf("older provenance = %v, want session-1", revisions[1].SourceSessionID)
	}
}

func TestDocumentServiceCoalescesSameSourceWithinWindow(t *testing.T) {
	svc, repo := newDocumentTestService(t)
	seedDocumentSession(t, repo, "task-doc", "session-1")
	ctx := context.Background()
	provenance := DocumentWriteProvenance{SourceTaskID: "task-doc", SourceSessionID: "session-1", SourceWorkflowStepID: "step-1"}

	if _, err := svc.CreateOrUpdateDocumentWithProvenance(ctx, "task-doc", "spec", "custom", "Spec", "v1", "agent", "Agent", provenance); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if _, err := svc.CreateOrUpdateDocumentWithProvenance(ctx, "task-doc", "spec", "custom", "Spec", "v2", "agent", "Agent", provenance); err != nil {
		t.Fatalf("second write: %v", err)
	}

	revisions, err := svc.ListRevisions(ctx, "task-doc", "spec", 0)
	if err != nil {
		t.Fatalf("list revisions: %v", err)
	}
	if len(revisions) != 1 {
		t.Fatalf("revision count = %d, want 1 (same source coalesces)", len(revisions))
	}
	if revisions[0].SourceSessionID == nil || *revisions[0].SourceSessionID != "session-1" {
		t.Fatalf("coalesced provenance = %v, want session-1", revisions[0].SourceSessionID)
	}
}
