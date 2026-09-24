package models

import "time"

// RoutingOutcome is the committed result of one workflow session-routing
// decision: whether the runtime reused a session, created one, or declined to
// route. The enum is closed; persistence stores the exact string.
type RoutingOutcome string

const (
	// RoutingOutcomeReused means an existing session was selected as the
	// destination for the workflow entry.
	RoutingOutcomeReused RoutingOutcome = "reused"
	// RoutingOutcomeCreated means no reusable session existed (or policy
	// forced one), so a fresh session was created.
	RoutingOutcomeCreated RoutingOutcome = "created"
	// RoutingOutcomeDeclined means routing did not select or create a
	// session, for example because a candidate was incompatible.
	RoutingOutcomeDeclined RoutingOutcome = "declined"
)

// RoutingReason is the closed-set explanation for a RoutingOutcome. The
// frontend must render the code, never parse a free-form string.
type RoutingReason string

const (
	RoutingReasonReusedCurrentSession      RoutingReason = "reused_current_session"
	RoutingReasonReusedExisting            RoutingReason = "reused_existing"
	RoutingReasonExplicitTarget            RoutingReason = "explicit_target"
	RoutingReasonForcedNewPolicy           RoutingReason = "forced_new_policy"
	RoutingReasonNoReusableCandidate       RoutingReason = "no_reusable_candidate"
	RoutingReasonExactModelIncompatibility RoutingReason = "exact_model_incompatibility"
	RoutingReasonSelectedCandidateTerminal RoutingReason = "selected_candidate_terminal"
)

// TaskSessionRoute is one durable row in the task session-route ledger. Unlike
// the bounded latest-route task metadata (MetaKeyWorkflowSessionRoute), this
// ledger grows one row per committed routing decision so task history can be
// reconstructed after a session is deleted or superseded.
//
// SourceSessionID and DestinationSessionID are nullable: deleting a session
// must not erase the routing history. A nil pointer renders as Unknown/legacy.
type TaskSessionRoute struct {
	ID                        string         `json:"id" db:"id"`
	TaskID                    string         `json:"task_id" db:"task_id"`
	DestinationWorkflowStepID string         `json:"destination_workflow_step_id" db:"destination_workflow_step_id"`
	SourceSessionID           *string        `json:"source_session_id,omitempty" db:"source_session_id"`
	DestinationSessionID      *string        `json:"destination_session_id,omitempty" db:"destination_session_id"`
	AgentProfileID            string         `json:"agent_profile_id,omitempty" db:"agent_profile_id"`
	StartPolicy               string         `json:"start_policy,omitempty" db:"start_policy"`
	EndPolicy                 string         `json:"end_policy,omitempty" db:"end_policy"`
	Outcome                   RoutingOutcome `json:"outcome" db:"outcome"`
	Reason                    RoutingReason  `json:"reason" db:"reason"`
	WorkflowStepTransitionID  *int64         `json:"workflow_step_transition_id,omitempty" db:"workflow_step_transition_id"`
	// CorrelationID makes the write idempotent: recording the same routing
	// decision twice for one task is a no-op.
	CorrelationID string    `json:"correlation_id,omitempty" db:"correlation_id"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
}
