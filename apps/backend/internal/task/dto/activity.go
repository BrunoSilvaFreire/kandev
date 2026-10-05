package dto

import (
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// TaskStepTransitionDTO is one committed movement-ledger row.
type TaskStepTransitionDTO struct {
	ID            int64     `json:"id"`
	SessionID     *string   `json:"session_id,omitempty"`
	FromStepID    string    `json:"from_step_id,omitempty"`
	ToStepID      string    `json:"to_step_id,omitempty"`
	Trigger       string    `json:"trigger"`
	TriggerDetail string    `json:"trigger_detail,omitempty"`
	ActorKind     string    `json:"actor_kind"`
	OccurredAt    time.Time `json:"occurred_at"`
}

// TaskActivitySessionDTO is the bounded session projection on an activity row.
type TaskActivitySessionDTO struct {
	ID             string     `json:"id"`
	Name           string     `json:"name,omitempty"`
	AgentProfileID string     `json:"agent_profile_id,omitempty"`
	State          string     `json:"state"`
	StartedAt      time.Time  `json:"started_at"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
}

// TaskActivityRouteDTO is one durable routing decision.
type TaskActivityRouteDTO struct {
	ID                       string    `json:"id"`
	DestinationStepID        string    `json:"destination_step_id"`
	SourceSessionID          *string   `json:"source_session_id,omitempty"`
	DestinationSessionID     *string   `json:"destination_session_id,omitempty"`
	AgentProfileID           string    `json:"agent_profile_id,omitempty"`
	StartPolicy              string    `json:"start_policy,omitempty"`
	EndPolicy                string    `json:"end_policy,omitempty"`
	Outcome                  string    `json:"outcome"`
	Reason                   string    `json:"reason"`
	DecisionDetail           *string   `json:"decision_detail,omitempty"`
	WorkflowStepTransitionID *int64    `json:"workflow_step_transition_id,omitempty"`
	CreatedAt                time.Time `json:"created_at"`
}

// TaskActivityDocumentRevisionDTO is revision metadata (no body).
type TaskActivityDocumentRevisionDTO struct {
	ID                   string    `json:"id"`
	DocumentKey          string    `json:"document_key"`
	RevisionNumber       int       `json:"revision_number"`
	Title                string    `json:"title"`
	AuthorKind           string    `json:"author_kind"`
	AuthorName           string    `json:"author_name"`
	SourceSessionID      *string   `json:"source_session_id,omitempty"`
	SourceWorkflowStepID *string   `json:"source_workflow_step_id,omitempty"`
	CreatedAt            time.Time `json:"created_at"`
}

// TaskActivityReviewRunDTO is one native review run.
type TaskActivityReviewRunDTO struct {
	ID           string     `json:"id"`
	SessionID    string     `json:"session_id,omitempty"`
	Status       string     `json:"status"`
	Trigger      string     `json:"trigger,omitempty"`
	Summary      string     `json:"summary,omitempty"`
	FindingCount int        `json:"finding_count"`
	CreatedAt    time.Time  `json:"created_at"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
}

// TaskActivityEventDTO is one entry in the merged activity stream.
type TaskActivityEventDTO struct {
	Kind             string                           `json:"kind"`
	ID               string                           `json:"id"`
	OccurredAt       time.Time                        `json:"occurred_at"`
	Transition       *TaskStepTransitionDTO           `json:"transition,omitempty"`
	Session          *TaskActivitySessionDTO          `json:"session,omitempty"`
	Route            *TaskActivityRouteDTO            `json:"route,omitempty"`
	DocumentRevision *TaskActivityDocumentRevisionDTO `json:"document_revision,omitempty"`
	ReviewRun        *TaskActivityReviewRunDTO        `json:"review_run,omitempty"`
}

// TaskActivityListResponse is one bounded activity page.
type TaskActivityListResponse struct {
	Events     []TaskActivityEventDTO `json:"events"`
	HasMore    bool                   `json:"has_more"`
	NextCursor string                 `json:"next_cursor,omitempty"`
}

// StepTransitionCountDTO is a task-local committed count for one directed pair.
type StepTransitionCountDTO struct {
	FromStepID string `json:"from_step_id"`
	ToStepID   string `json:"to_step_id"`
	Count      int    `json:"count"`
}

// StepVisitSessionDTO is one destination session of a step visit.
type StepVisitSessionDTO struct {
	SessionID      string    `json:"session_id"`
	AgentProfileID string    `json:"agent_profile_id,omitempty"`
	TransitionID   *int64    `json:"workflow_step_transition_id,omitempty"`
	OccurredAt     time.Time `json:"occurred_at"`
}

// StepVisitSummaryDTO is the task-local visit count for one destination step.
type StepVisitSummaryDTO struct {
	StepID   string                `json:"step_id"`
	Count    int                   `json:"count"`
	Sessions []StepVisitSessionDTO `json:"sessions"`
}

// TaskTransitionCountsResponse is the bounded header-count payload.
type TaskTransitionCountsResponse struct {
	Counts []StepTransitionCountDTO `json:"counts"`
	Visits []StepVisitSummaryDTO    `json:"visits"`
}

// TaskActivityEventFromModel maps one merged event to its wire shape.
func TaskActivityEventFromModel(event *models.TaskActivityEvent) TaskActivityEventDTO {
	out := TaskActivityEventDTO{
		Kind:       string(event.Kind),
		ID:         event.ID,
		OccurredAt: event.OccurredAt,
	}
	if event.Transition != nil {
		out.Transition = &TaskStepTransitionDTO{
			ID:            event.Transition.ID,
			SessionID:     event.Transition.SessionID,
			FromStepID:    event.Transition.FromWorkflowStepID,
			ToStepID:      event.Transition.ToWorkflowStepID,
			Trigger:       event.Transition.Trigger,
			TriggerDetail: event.Transition.TriggerDetail,
			ActorKind:     event.Transition.ActorKind,
			OccurredAt:    event.Transition.OccurredAt,
		}
	}
	if event.Session != nil {
		out.Session = &TaskActivitySessionDTO{
			ID:             event.Session.ID,
			Name:           event.Session.Name,
			AgentProfileID: event.Session.AgentProfileID,
			State:          string(event.Session.State),
			StartedAt:      event.Session.StartedAt,
			CompletedAt:    event.Session.CompletedAt,
		}
	}
	if event.Route != nil {
		out.Route = &TaskActivityRouteDTO{
			ID:                       event.Route.ID,
			DestinationStepID:        event.Route.DestinationWorkflowStepID,
			SourceSessionID:          event.Route.SourceSessionID,
			DestinationSessionID:     event.Route.DestinationSessionID,
			AgentProfileID:           event.Route.AgentProfileID,
			StartPolicy:              event.Route.StartPolicy,
			EndPolicy:                event.Route.EndPolicy,
			Outcome:                  string(event.Route.Outcome),
			Reason:                   string(event.Route.Reason),
			DecisionDetail:           event.Route.DecisionDetail,
			WorkflowStepTransitionID: event.Route.WorkflowStepTransitionID,
			CreatedAt:                event.Route.CreatedAt,
		}
	}
	if event.DocumentRevision != nil {
		out.DocumentRevision = &TaskActivityDocumentRevisionDTO{
			ID:                   event.DocumentRevision.ID,
			DocumentKey:          event.DocumentRevision.DocumentKey,
			RevisionNumber:       event.DocumentRevision.RevisionNumber,
			Title:                event.DocumentRevision.Title,
			AuthorKind:           event.DocumentRevision.AuthorKind,
			AuthorName:           event.DocumentRevision.AuthorName,
			SourceSessionID:      event.DocumentRevision.SourceSessionID,
			SourceWorkflowStepID: event.DocumentRevision.SourceWorkflowStepID,
			CreatedAt:            event.DocumentRevision.CreatedAt,
		}
	}
	if event.ReviewRun != nil {
		out.ReviewRun = &TaskActivityReviewRunDTO{
			ID:           event.ReviewRun.ID,
			SessionID:    event.ReviewRun.SessionID,
			Status:       string(event.ReviewRun.Status),
			Trigger:      string(event.ReviewRun.Trigger),
			Summary:      event.ReviewRun.Summary,
			FindingCount: event.ReviewRun.FindingCount,
			CreatedAt:    event.ReviewRun.CreatedAt,
			CompletedAt:  event.ReviewRun.CompletedAt,
		}
	}
	return out
}
