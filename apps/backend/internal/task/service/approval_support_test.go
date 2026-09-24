package service

import (
	"testing"

	"github.com/kandev/kandev/internal/task/models"
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
