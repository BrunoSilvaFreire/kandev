package dto

import (
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
)

// TaskDocumentSummaryDTO is one metadata-only catalog row. It never carries a
// document body; callers fetch content only after selecting an entry.
type TaskDocumentSummaryDTO struct {
	Key                  string    `json:"key"`
	Title                string    `json:"title"`
	Type                 string    `json:"type"`
	IsPlan               bool      `json:"is_plan"`
	LatestRevisionNumber int       `json:"latest_revision_number"`
	UpdatedAt            time.Time `json:"updated_at"`
	SourceSessionID      *string   `json:"source_session_id,omitempty"`
	SourceWorkflowStepID *string   `json:"source_workflow_step_id,omitempty"`
}

// TaskDocumentCatalogGroupDTO groups catalog entries by the session that
// produced their latest revision. A nil SessionID is the Unknown/legacy group.
type TaskDocumentCatalogGroupDTO struct {
	SessionID *string                  `json:"session_id,omitempty"`
	Entries   []TaskDocumentSummaryDTO `json:"entries"`
}

// TaskDocumentDetailDTO is a selected document with its content and the
// latest revision's provenance.
type TaskDocumentDetailDTO struct {
	Key                  string    `json:"key"`
	Title                string    `json:"title"`
	Type                 string    `json:"type"`
	IsPlan               bool      `json:"is_plan"`
	Content              string    `json:"content"`
	LatestRevisionNumber int       `json:"latest_revision_number"`
	UpdatedAt            time.Time `json:"updated_at"`
	SourceSessionID      *string   `json:"source_session_id,omitempty"`
	SourceWorkflowStepID *string   `json:"source_workflow_step_id,omitempty"`
}

// TaskDocumentRevisionDTO is one revision in a document's history. Content is
// omitted from list responses and included for a single revision fetch.
type TaskDocumentRevisionDTO struct {
	ID                   string    `json:"id"`
	RevisionNumber       int       `json:"revision_number"`
	Title                string    `json:"title"`
	Content              string    `json:"content,omitempty"`
	AuthorKind           string    `json:"author_kind"`
	AuthorName           string    `json:"author_name"`
	SourceSessionID      *string   `json:"source_session_id,omitempty"`
	SourceWorkflowStepID *string   `json:"source_workflow_step_id,omitempty"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// DocumentCatalogGroupFromService converts one grouped catalog row.
func DocumentCatalogGroupFromService(group service.DocumentCatalogGroup) TaskDocumentCatalogGroupDTO {
	entries := make([]TaskDocumentSummaryDTO, 0, len(group.Entries))
	for _, entry := range group.Entries {
		entries = append(entries, DocumentSummaryFromService(entry))
	}
	return TaskDocumentCatalogGroupDTO{SessionID: group.SessionID, Entries: entries}
}

// DocumentSummaryFromService converts one catalog entry.
func DocumentSummaryFromService(entry service.DocumentCatalogEntry) TaskDocumentSummaryDTO {
	return TaskDocumentSummaryDTO{
		Key:                  entry.Key,
		Title:                entry.Title,
		Type:                 entry.Type,
		IsPlan:               entry.IsPlan,
		LatestRevisionNumber: entry.LatestRevisionNumber,
		UpdatedAt:            entry.UpdatedAt,
		SourceSessionID:      entry.SourceSessionID,
		SourceWorkflowStepID: entry.SourceWorkflowStepID,
	}
}

// TaskDocumentRevisionFromModel converts a general document revision.
func TaskDocumentRevisionFromModel(rev *models.TaskDocumentRevision, includeContent bool) *TaskDocumentRevisionDTO {
	if rev == nil {
		return nil
	}
	out := &TaskDocumentRevisionDTO{
		ID:                   rev.ID,
		RevisionNumber:       rev.RevisionNumber,
		Title:                rev.Title,
		AuthorKind:           rev.AuthorKind,
		AuthorName:           rev.AuthorName,
		SourceSessionID:      rev.SourceSessionID,
		SourceWorkflowStepID: rev.SourceWorkflowStepID,
		CreatedAt:            rev.CreatedAt,
		UpdatedAt:            rev.UpdatedAt,
	}
	if includeContent {
		out.Content = rev.Content
	}
	return out
}

// TaskDocumentRevisionFromPlanRevision converts a plan revision into the
// shared revision shape. Plan revisions carry a workflow-step snapshot but no
// session provenance, so SourceSessionID is always nil.
func TaskDocumentRevisionFromPlanRevision(rev *models.TaskPlanRevision, includeContent bool) *TaskDocumentRevisionDTO {
	if rev == nil {
		return nil
	}
	out := &TaskDocumentRevisionDTO{
		ID:             rev.ID,
		RevisionNumber: rev.RevisionNumber,
		Title:          rev.Title,
		AuthorKind:     rev.AuthorKind,
		AuthorName:     rev.AuthorName,
		CreatedAt:      rev.CreatedAt,
		UpdatedAt:      rev.UpdatedAt,
	}
	if rev.WorkflowStepID != "" {
		stepID := rev.WorkflowStepID
		out.SourceWorkflowStepID = &stepID
	}
	if includeContent {
		out.Content = rev.Content
	}
	return out
}

// DocumentDetailFromModels builds the detail DTO for a general document.
func DocumentDetailFromModels(doc *models.TaskDocument, latest *models.TaskDocumentRevision) TaskDocumentDetailDTO {
	out := TaskDocumentDetailDTO{
		Key:       doc.Key,
		Title:     doc.Title,
		Type:      doc.Type,
		Content:   doc.Content,
		UpdatedAt: doc.UpdatedAt,
	}
	if latest != nil {
		out.LatestRevisionNumber = latest.RevisionNumber
		out.SourceSessionID = latest.SourceSessionID
		out.SourceWorkflowStepID = latest.SourceWorkflowStepID
	}
	return out
}

// PlanDetailFromModel builds the detail DTO for the synthetic Plan entry.
func PlanDetailFromModel(plan *models.TaskPlan, latest *models.TaskPlanRevision) TaskDocumentDetailDTO {
	out := TaskDocumentDetailDTO{
		Key:       service.PlanDocumentKey,
		Title:     plan.Title,
		Type:      "plan",
		IsPlan:    true,
		Content:   plan.Content,
		UpdatedAt: plan.UpdatedAt,
	}
	if latest != nil {
		out.LatestRevisionNumber = latest.RevisionNumber
		if latest.WorkflowStepID != "" {
			stepID := latest.WorkflowStepID
			out.SourceWorkflowStepID = &stepID
		}
	}
	return out
}
