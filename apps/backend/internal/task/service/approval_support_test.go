package service

import (
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/plancomments"
)

func TestResolveApprovalCommentsValidatesExactRefs(t *testing.T) {
	snapshot := &models.TaskPlanCommentSnapshot{
		TaskID: "task-a",
		PlanID: "plan-a",
		Comments: []*models.TaskPlanComment{
			{ID: "c1", TaskID: "task-a", Version: 2},
			{ID: "c2", TaskID: "task-a", Version: 5},
		},
	}
	comments, ids, ok := resolveApprovalComments(snapshot, []models.TaskPlanCommentRef{
		{ID: "c2", Version: 5},
		{ID: "c1", Version: 2},
	})
	if !ok || len(comments) != 2 || len(ids) != 2 {
		t.Fatalf("expected both comments resolved, got ok=%v comments=%d ids=%d", ok, len(comments), len(ids))
	}
	// Ordering is by id when created_at is equal.
	if ids[0] != "c1" || ids[1] != "c2" {
		t.Fatalf("unexpected ordering: %v", ids)
	}

	// A ref from another task is simply absent from this task's snapshot.
	if _, _, ok := resolveApprovalComments(snapshot, []models.TaskPlanCommentRef{{ID: "foreign", Version: 1}}); ok {
		t.Fatalf("foreign ref must not resolve")
	}
	// A stale version must not resolve.
	if _, _, ok := resolveApprovalComments(snapshot, []models.TaskPlanCommentRef{{ID: "c1", Version: 99}}); ok {
		t.Fatalf("stale version must not resolve")
	}
	// A duplicate ref must not resolve.
	if _, _, ok := resolveApprovalComments(snapshot, []models.TaskPlanCommentRef{
		{ID: "c1", Version: 2}, {ID: "c1", Version: 2},
	}); ok {
		t.Fatalf("duplicate ref must not resolve")
	}
	// Empty refs never resolve.
	if _, _, ok := resolveApprovalComments(snapshot, nil); ok {
		t.Fatalf("empty refs must not resolve")
	}
}

// TestApprovalCommentsBatchIntoOneRevisionBlock locks in Package 4's batching
// contract: every recorded plan comment referenced by one approval answer is
// rendered as a single canonical block and consumed as one batch. Consuming
// the batch is feedback delivery, never plan approval; enrollment still needs
// a separate winning human approve receipt (covered by the completion-gate
// repository tests).
func TestApprovalCommentsBatchIntoOneRevisionBlock(t *testing.T) {
	snapshot := &models.TaskPlanCommentSnapshot{
		TaskID: "task-a",
		PlanID: "plan-a",
		Comments: []*models.TaskPlanComment{
			{ID: "c2", TaskID: "task-a", Version: 1, Body: "second comment"},
			{ID: "c1", TaskID: "task-a", Version: 1, Body: "first comment"},
		},
	}
	comments, ids, ok := resolveApprovalComments(snapshot, []models.TaskPlanCommentRef{
		{ID: "c1", Version: 1},
		{ID: "c2", Version: 1},
	})
	if !ok || len(ids) != 2 || ids[0] != "c1" || ids[1] != "c2" {
		t.Fatalf("expected one deterministic batch of both comments, got ok=%v ids=%v", ok, ids)
	}

	block := plancomments.FormatComments(comments)
	if strings.Count(block, "### Plan Comments") != 1 {
		t.Fatalf("expected exactly one batched block header, got:\n%s", block)
	}
	if !strings.Contains(block, "> first comment") || !strings.Contains(block, "> second comment") {
		t.Fatalf("expected both comments in the batched block, got:\n%s", block)
	}
}
