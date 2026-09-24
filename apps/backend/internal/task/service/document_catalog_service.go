package service

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
)

// PlanDocumentKey is the reserved document key for the synthetic, authoritative
// Plan entry in the task document catalog. A general document with this key is
// always shadowed by the plan stored in task_plans.
const PlanDocumentKey = "plan"

// documentCatalogRepo is the metadata surface the catalog needs. It never loads
// document bodies: the HEAD list and a one-row revision window per document.
type documentCatalogRepo interface {
	ListDocuments(ctx context.Context, taskID string) ([]*models.TaskDocument, error)
	ListRevisions(ctx context.Context, taskID, key string, limit int) ([]*models.TaskDocumentRevision, error)
}

// planCatalogReader is the read surface for the synthetic Plan entry.
type planCatalogReader interface {
	GetPlan(ctx context.Context, taskID string) (*models.TaskPlan, error)
	ListRevisions(ctx context.Context, taskID string) ([]*models.TaskPlanRevision, error)
}

// DocumentCatalogEntry is one metadata-only catalog row. Content is never part
// of a catalog response; callers load it only after a selection.
type DocumentCatalogEntry struct {
	Key                  string    `json:"key"`
	Title                string    `json:"title"`
	Type                 string    `json:"type"`
	IsPlan               bool      `json:"is_plan"`
	LatestRevisionNumber int       `json:"latest_revision_number"`
	UpdatedAt            time.Time `json:"updated_at"`
	// SourceSessionID and SourceWorkflowStepID are the latest revision's
	// producing provenance. Nil means Unknown/legacy: the fields are never
	// inferred from timestamps or the task's current step.
	SourceSessionID      *string `json:"source_session_id,omitempty"`
	SourceWorkflowStepID *string `json:"source_workflow_step_id,omitempty"`
}

// DocumentCatalogGroup collects entries whose latest revision came from the
// same producing session. A nil SessionID is the Unknown/legacy group.
type DocumentCatalogGroup struct {
	SessionID *string                `json:"session_id,omitempty"`
	Entries   []DocumentCatalogEntry `json:"entries"`
}

// DocumentCatalogService merges the authoritative Plan with general task
// documents into one metadata-only catalog.
type DocumentCatalogService struct {
	docs  documentCatalogRepo
	plans planCatalogReader
	log   *logger.Logger
}

// NewDocumentCatalogService creates a DocumentCatalogService.
func NewDocumentCatalogService(docs documentCatalogRepo, plans planCatalogReader, log *logger.Logger) *DocumentCatalogService {
	return &DocumentCatalogService{docs: docs, plans: plans, log: log}
}

// Catalog returns the merged, metadata-only catalog. The synthetic Plan entry
// wins any document_key collision. Entries are ordered plan-first then by key.
func (s *DocumentCatalogService) Catalog(ctx context.Context, taskID string) ([]DocumentCatalogEntry, error) {
	if taskID == "" {
		return nil, ErrDocumentTaskRequired
	}
	entries := make([]DocumentCatalogEntry, 0)

	if planEntry, err := s.planEntry(ctx, taskID); err != nil {
		return nil, err
	} else if planEntry != nil {
		entries = append(entries, *planEntry)
	}

	docs, err := s.docs.ListDocuments(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("list documents for catalog: %w", err)
	}
	for _, doc := range docs {
		if doc == nil || doc.Key == PlanDocumentKey {
			// The synthetic Plan entry is authoritative for this key.
			continue
		}
		entry, err := s.documentEntry(ctx, doc)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}

	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].IsPlan != entries[j].IsPlan {
			return entries[i].IsPlan
		}
		return entries[i].Key < entries[j].Key
	})
	return entries, nil
}

// CatalogGrouped returns the catalog grouped by latest-revision producing
// session, preserving the catalog's stable order within each group. Groups are
// ordered by first appearance, so the plan-first ordering survives.
func (s *DocumentCatalogService) CatalogGrouped(ctx context.Context, taskID string) ([]DocumentCatalogGroup, error) {
	entries, err := s.Catalog(ctx, taskID)
	if err != nil {
		return nil, err
	}
	groups := make([]DocumentCatalogGroup, 0)
	index := make(map[string]int)
	for _, entry := range entries {
		key := legacySessionGroupKey
		if entry.SourceSessionID != nil {
			key = *entry.SourceSessionID
		}
		position, ok := index[key]
		if !ok {
			groups = append(groups, DocumentCatalogGroup{SessionID: entry.SourceSessionID})
			position = len(groups) - 1
			index[key] = position
		}
		groups[position].Entries = append(groups[position].Entries, entry)
	}
	return groups, nil
}

const legacySessionGroupKey = "\x00legacy"

func (s *DocumentCatalogService) planEntry(ctx context.Context, taskID string) (*DocumentCatalogEntry, error) {
	plan, err := s.plans.GetPlan(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("get plan for catalog: %w", err)
	}
	if plan == nil {
		return nil, nil
	}
	entry := &DocumentCatalogEntry{
		Key:       PlanDocumentKey,
		Title:     plan.Title,
		Type:      "plan",
		IsPlan:    true,
		UpdatedAt: plan.UpdatedAt,
	}
	revisions, err := s.plans.ListRevisions(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("list plan revisions for catalog: %w", err)
	}
	for _, rev := range revisions {
		if rev == nil || rev.RevisionNumber < entry.LatestRevisionNumber {
			continue
		}
		entry.LatestRevisionNumber = rev.RevisionNumber
		if rev.WorkflowStepID != "" {
			stepID := rev.WorkflowStepID
			entry.SourceWorkflowStepID = &stepID
		}
	}
	return entry, nil
}

func (s *DocumentCatalogService) documentEntry(ctx context.Context, doc *models.TaskDocument) (DocumentCatalogEntry, error) {
	entry := DocumentCatalogEntry{
		Key:       doc.Key,
		Title:     doc.Title,
		Type:      doc.Type,
		UpdatedAt: doc.UpdatedAt,
	}
	revisions, err := s.docs.ListRevisions(ctx, doc.TaskID, doc.Key, 1)
	if err != nil {
		return entry, fmt.Errorf("latest revision for document %q: %w", doc.Key, err)
	}
	if len(revisions) > 0 && revisions[0] != nil {
		latest := revisions[0]
		entry.LatestRevisionNumber = latest.RevisionNumber
		entry.SourceSessionID = latest.SourceSessionID
		entry.SourceWorkflowStepID = latest.SourceWorkflowStepID
	}
	return entry, nil
}
