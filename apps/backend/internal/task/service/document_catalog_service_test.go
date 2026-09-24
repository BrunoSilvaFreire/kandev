package service

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

type fakeCatalogRepo struct {
	docs      []*models.TaskDocument
	revisions map[string][]*models.TaskDocumentRevision
}

func (f *fakeCatalogRepo) ListDocuments(_ context.Context, _ string) ([]*models.TaskDocument, error) {
	return f.docs, nil
}

func (f *fakeCatalogRepo) ListRevisions(_ context.Context, _ string, key string, limit int) ([]*models.TaskDocumentRevision, error) {
	revs := f.revisions[key]
	if limit > 0 && len(revs) > limit {
		return revs[:limit], nil
	}
	return revs, nil
}

type fakePlanCatalogReader struct {
	plan      *models.TaskPlan
	revisions []*models.TaskPlanRevision
}

func (f *fakePlanCatalogReader) GetPlan(_ context.Context, _ string) (*models.TaskPlan, error) {
	return f.plan, nil
}

func (f *fakePlanCatalogReader) ListRevisions(_ context.Context, _ string) ([]*models.TaskPlanRevision, error) {
	return f.revisions, nil
}

func strPtr(v string) *string { return &v }

func TestDocumentCatalog_MergesPlanFirstAndMetadataOnly(t *testing.T) {
	now := time.Now().UTC()
	sess := "sess-alpha"
	step := "step-impl"
	repo := &fakeCatalogRepo{
		docs: []*models.TaskDocument{
			{ID: "d1", TaskID: "t1", Key: "architecture", Type: "custom", Title: "Architecture", Content: "secret body", UpdatedAt: now},
			{ID: "d2", TaskID: "t1", Key: "plan", Type: "plan", Title: "Shadowed plan", Content: "shadow", UpdatedAt: now},
		},
		revisions: map[string][]*models.TaskDocumentRevision{
			"architecture": {
				{ID: "r1", TaskID: "t1", DocumentKey: "architecture", RevisionNumber: 3, SourceSessionID: &sess, SourceWorkflowStepID: &step, CreatedAt: now, UpdatedAt: now},
				{ID: "r0", TaskID: "t1", DocumentKey: "architecture", RevisionNumber: 2, CreatedAt: now, UpdatedAt: now},
			},
		},
	}
	plans := &fakePlanCatalogReader{
		plan:      &models.TaskPlan{ID: "p1", TaskID: "t1", Title: "Plan", UpdatedAt: now},
		revisions: []*models.TaskPlanRevision{{ID: "pr2", TaskID: "t1", RevisionNumber: 2, WorkflowStepID: "step-plan"}},
	}
	svc := NewDocumentCatalogService(repo, plans, nil)

	entries, err := svc.Catalog(context.Background(), "t1")
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2 (shadowed plan excluded)", len(entries))
	}
	if !entries[0].IsPlan || entries[0].Key != "plan" {
		t.Fatalf("first entry = %+v, want synthetic plan first", entries[0])
	}
	if entries[0].LatestRevisionNumber != 2 {
		t.Fatalf("plan revision number = %d, want 2", entries[0].LatestRevisionNumber)
	}
	if entries[0].SourceWorkflowStepID == nil || *entries[0].SourceWorkflowStepID != "step-plan" {
		t.Fatalf("plan source step = %v, want step-plan", entries[0].SourceWorkflowStepID)
	}
	arch := entries[1]
	if arch.Key != "architecture" || arch.LatestRevisionNumber != 3 {
		t.Fatalf("architecture entry = %+v", arch)
	}
	if arch.SourceSessionID == nil || *arch.SourceSessionID != "sess-alpha" {
		t.Fatalf("architecture source session = %v, want sess-alpha", arch.SourceSessionID)
	}
}

func TestDocumentCatalog_GroupedByProducingSession(t *testing.T) {
	now := time.Now().UTC()
	alpha := "sess-alpha"
	repo := &fakeCatalogRepo{
		docs: []*models.TaskDocument{
			{ID: "d1", TaskID: "t1", Key: "alpha-doc", Type: "custom", Title: "Alpha", UpdatedAt: now},
			{ID: "d2", TaskID: "t1", Key: "legacy-doc", Type: "custom", Title: "Legacy", UpdatedAt: now},
		},
		revisions: map[string][]*models.TaskDocumentRevision{
			"alpha-doc":  {{ID: "r1", RevisionNumber: 1, SourceSessionID: &alpha}},
			"legacy-doc": {{ID: "r2", RevisionNumber: 1}},
		},
	}
	plans := &fakePlanCatalogReader{}
	svc := NewDocumentCatalogService(repo, plans, nil)

	groups, err := svc.CatalogGrouped(context.Background(), "t1")
	if err != nil {
		t.Fatalf("catalog grouped: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("groups = %d, want 2", len(groups))
	}
	if groups[0].SessionID == nil || *groups[0].SessionID != "sess-alpha" {
		t.Fatalf("first group session = %v, want sess-alpha", groups[0].SessionID)
	}
	if groups[1].SessionID != nil {
		t.Fatalf("legacy group session = %v, want nil", groups[1].SessionID)
	}
}

func TestDocumentCatalog_RequiresTaskID(t *testing.T) {
	svc := NewDocumentCatalogService(&fakeCatalogRepo{}, &fakePlanCatalogReader{}, nil)
	if _, err := svc.Catalog(context.Background(), ""); err != ErrDocumentTaskRequired {
		t.Fatalf("err = %v, want ErrDocumentTaskRequired", err)
	}
}
