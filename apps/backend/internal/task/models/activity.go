package models

import "time"

// TaskStepTransition is one committed row of the authoritative task movement
// ledger. SessionID is the transition's initiator, never the destination
// session; destinations come from session creation provenance and the route
// ledger.
type TaskStepTransition struct {
	ID                 int64     `json:"id" db:"id"`
	TaskID             string    `json:"task_id" db:"task_id"`
	SessionID          *string   `json:"session_id,omitempty" db:"session_id"`
	FromWorkflowID     string    `json:"from_workflow_id,omitempty" db:"from_workflow_id"`
	FromWorkflowStepID string    `json:"from_workflow_step_id,omitempty" db:"from_workflow_step_id"`
	ToWorkflowID       string    `json:"to_workflow_id,omitempty" db:"to_workflow_id"`
	ToWorkflowStepID   string    `json:"to_workflow_step_id,omitempty" db:"to_workflow_step_id"`
	Trigger            string    `json:"trigger" db:"trigger"`
	ActorKind          string    `json:"actor_kind" db:"actor_kind"`
	ActorID            string    `json:"actor_id,omitempty" db:"actor_id"`
	TriggerDetail      string    `json:"trigger_detail,omitempty" db:"trigger_detail"`
	ContractVersion    int       `json:"contract_version" db:"contract_version"`
	OccurredAt         time.Time `json:"occurred_at" db:"occurred_at"`
}

// StepPair is a directed source→destination step pair used to aggregate
// committed transition counts.
type StepPair struct {
	FromStepID string
	ToStepID   string
}

// StepTransitionCount is the task-local count of successfully committed
// transitions for one directed step pair.
type StepTransitionCount struct {
	FromStepID string `json:"from_step_id"`
	ToStepID   string `json:"to_step_id"`
	Count      int    `json:"count"`
}

// TaskActivityKind discriminates the typed activity union.
type TaskActivityKind string

// Activity kinds projected over the authoritative tables.
const (
	ActivityKindTransition       TaskActivityKind = "transition"
	ActivityKindSessionCreated   TaskActivityKind = "session_created"
	ActivityKindSessionCompleted TaskActivityKind = "session_completed"
	ActivityKindRoute            TaskActivityKind = "route"
	ActivityKindDocumentRevision TaskActivityKind = "document_revision"
	ActivityKindReviewRun        TaskActivityKind = "review_run"
)

// TaskActivityEvent is one entry in the merged, chronological task activity
// stream. Exactly one payload pointer is set, matching Kind.
type TaskActivityEvent struct {
	Kind             TaskActivityKind      `json:"kind"`
	ID               string                `json:"id"`
	OccurredAt       time.Time             `json:"occurred_at"`
	Transition       *TaskStepTransition   `json:"transition,omitempty"`
	Session          *TaskSession          `json:"session,omitempty"`
	Route            *TaskSessionRoute     `json:"route,omitempty"`
	DocumentRevision *TaskDocumentRevision `json:"document_revision,omitempty"`
	ReviewRun        *TaskReviewRun        `json:"review_run,omitempty"`
}

// TaskActivityFilter bounds one activity page.
type TaskActivityFilter struct {
	// StepID narrows the stream to step visits (transitions whose destination
	// is this step) when non-empty.
	StepID string
	// Before is the exclusive keyset cursor (page 1 when nil).
	Before *time.Time
	Limit  int
}

// StepVisitSession is one persisted destination session of a step visit. It is
// derived from the route ledger: TransitionID is the movement row that produced
// the visit and OccurredAt orders sessions oldest to newest.
type StepVisitSession struct {
	SessionID      string    `json:"session_id"`
	AgentProfileID string    `json:"agent_profile_id,omitempty"`
	TransitionID   *int64    `json:"workflow_step_transition_id,omitempty"`
	OccurredAt     time.Time `json:"occurred_at"`
}

// StepVisitSummary is the task-local visit (entry) count for one destination
// step plus its ordered, session-deduplicated destination sessions. Sessions
// come only from the route ledger; the transition's initiating session is
// never a destination.
type StepVisitSummary struct {
	StepID   string             `json:"step_id"`
	Count    int                `json:"count"`
	Sessions []StepVisitSession `json:"sessions"`
}

// TaskTransitionSummary is the bounded header payload: committed directed-pair
// counts plus per-step visit counts.
type TaskTransitionSummary struct {
	Counts []StepTransitionCount `json:"counts"`
	Visits []StepVisitSummary    `json:"visits"`
}
