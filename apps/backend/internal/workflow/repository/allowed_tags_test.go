package repository

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/workflow/models"
)

func TestWorkflowStepAllowedTagsRoundTrip(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	step := &models.WorkflowStep{
		WorkflowID:  "wf-test",
		Name:        "Review",
		Position:    1,
		AllowedTags: []string{"review", "security"},
	}
	if err := repo.CreateStep(ctx, step); err != nil {
		t.Fatalf("create step: %v", err)
	}

	got, err := repo.GetStep(ctx, step.ID)
	if err != nil {
		t.Fatalf("get step: %v", err)
	}
	if len(got.AllowedTags) != 2 || got.AllowedTags[0] != "review" || got.AllowedTags[1] != "security" {
		t.Fatalf("allowed tags = %#v, want [review security]", got.AllowedTags)
	}

	got.AllowedTags = []string{"ops"}
	if err := repo.UpdateStep(ctx, got); err != nil {
		t.Fatalf("update step: %v", err)
	}
	reloaded, err := repo.GetStep(ctx, step.ID)
	if err != nil {
		t.Fatalf("reload step: %v", err)
	}
	if len(reloaded.AllowedTags) != 1 || reloaded.AllowedTags[0] != "ops" {
		t.Fatalf("allowed tags = %#v, want [ops]", reloaded.AllowedTags)
	}
}

// TestWorkflowStepAllowedTagsCanonicalizedAtRepositoryBoundary proves a direct
// repository write canonicalizes allowed tags even when it bypasses the
// service layer.
func TestWorkflowStepAllowedTagsCanonicalizedAtRepositoryBoundary(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	step := &models.WorkflowStep{
		WorkflowID:  "wf-test",
		Name:        "Review",
		Position:    1,
		AllowedTags: []string{" Review ", "security", "REVIEW"},
	}
	if err := repo.CreateStep(ctx, step); err != nil {
		t.Fatalf("create step: %v", err)
	}
	got, err := repo.GetStep(ctx, step.ID)
	if err != nil {
		t.Fatalf("get step: %v", err)
	}
	if len(got.AllowedTags) != 2 || got.AllowedTags[0] != "review" || got.AllowedTags[1] != "security" {
		t.Fatalf("allowed tags = %#v, want canonical [review security]", got.AllowedTags)
	}

	got.AllowedTags = []string{" Ops ", "ops", "build"}
	if err := repo.UpdateStep(ctx, got); err != nil {
		t.Fatalf("update step: %v", err)
	}
	reloaded, err := repo.GetStep(ctx, step.ID)
	if err != nil {
		t.Fatalf("reload step: %v", err)
	}
	if len(reloaded.AllowedTags) != 2 || reloaded.AllowedTags[0] != "build" || reloaded.AllowedTags[1] != "ops" {
		t.Fatalf("allowed tags = %#v, want canonical [build ops]", reloaded.AllowedTags)
	}
}

// TestWorkflowStepAllowedTagsRejectInvalidAtRepositoryBoundary proves a direct
// write rejects an oversized tag instead of persisting it.
func TestWorkflowStepAllowedTagsRejectInvalidAtRepositoryBoundary(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	step := &models.WorkflowStep{
		WorkflowID:  "wf-test",
		Name:        "Bad",
		Position:    1,
		AllowedTags: []string{strings.Repeat("x", 65)},
	}
	if err := repo.CreateStep(ctx, step); err == nil {
		t.Fatal("oversized allowed tag must be rejected at the repository boundary")
	}
}

func TestWorkflowStepAllowedTagsDefaultToEmpty(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	step := &models.WorkflowStep{WorkflowID: "wf-test", Name: "Work", Position: 1}
	if err := repo.CreateStep(ctx, step); err != nil {
		t.Fatalf("create step: %v", err)
	}
	got, err := repo.GetStep(ctx, step.ID)
	if err != nil {
		t.Fatalf("get step: %v", err)
	}
	if got.AllowedTags == nil || len(got.AllowedTags) != 0 {
		t.Fatalf("allowed tags = %#v, want empty non-nil", got.AllowedTags)
	}
}
