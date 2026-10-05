// Package clarification provides types and services for agent clarification requests.
// This allows agents to ask structured questions to users and wait for responses.
package clarification

import (
	"sync"
	"time"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

// Approval subjects supported by request_approval_kandev. A subject names
// what the user is being asked to approve.
const (
	ApprovalSubjectTaskPlan = "task_plan"
	ApprovalSubjectDocument = "document"
)

// Approval decisions. These are the stable option IDs of the fixed approval
// question, so older clients that render the bundle as a normal single
// question still produce a valid outcome.
const (
	ApprovalDecisionApprove = "approve"
	ApprovalDecisionRevise  = "revise"
	ApprovalDecisionReject  = "reject"
)

// ApprovalQuestionID is the fixed question id of an approval bundle.
const ApprovalQuestionID = "approval"

// ContinuationRecoveryQuestionID is the fixed question id of a
// continuation-recovery bundle.
const ContinuationRecoveryQuestionID = "continuation_recovery"

// Continuation-recovery decisions. These are the stable option IDs of the
// fixed recovery question.
const (
	ContinuationRecoveryDecisionRetry    = "retry"
	ContinuationRecoveryDecisionContinue = "continue"
)

// ContinuationRecoveryMeta is the request-side metadata that marks a
// clarification bundle as a continuation-recovery request. It is persisted
// alongside the bundle's question so the resolver can hand the decision to the
// orchestrator instead of delivering text to an agent.
type ContinuationRecoveryMeta struct {
	// Stamp is the CAS key of the pending_continuation record this bundle asks
	// about. A resolution whose stamp no longer matches is a no-op.
	Stamp string `json:"stamp"`
	// Case names the paused continuation path (cold | unavailable | quota_pressure).
	Case string `json:"case"`
	// Reason names why it paused (extraction_failed | reset_failed | no_available_profile).
	Reason string `json:"reason"`
}

// ApprovalMeta is the request-side metadata that marks a clarification bundle
// as an approval request. It is persisted alongside the bundle's questions so
// the resolver can render the subject's pending comments before claiming.
type ApprovalMeta struct {
	Subject          string `json:"subject"` // ApprovalSubjectTaskPlan | ApprovalSubjectDocument
	DocumentKey      string `json:"document_key,omitempty"`
	Title            string `json:"title"`
	VersionAtRequest string `json:"version_at_request,omitempty"`
}

// ApprovalOutcome is the response-side payload filled by the resolver when it
// resolves an approval bundle. It carries the decision, the user's free-text
// feedback, the rendered pending plan comments, the ids of the comments that
// were consumed, whether the user edited the subject since the request, and
// the subject's current version.
type ApprovalOutcome struct {
	Decision       string   `json:"decision"` // ApprovalDecisionApprove | Revise | Reject
	Feedback       string   `json:"feedback,omitempty"`
	PlanComments   string   `json:"plan_comments,omitempty"`
	CommentIDs     []string `json:"comment_ids,omitempty"`
	SubjectEdited  bool     `json:"subject_edited"`
	CurrentVersion string   `json:"current_version,omitempty"`
}

// Option represents a single choice option for a question.
type Option struct {
	ID          string `json:"option_id"`
	Label       string `json:"label"`       // Concise 1-5 words
	Description string `json:"description"` // Explanation of the option
}

// Question represents a single question with multiple choice options.
type Question struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`   // Short label (max 12 chars)
	Prompt  string   `json:"prompt"`  // Full question text
	Options []Option `json:"options"` // 2-6 options
}

// Request represents a clarification request from an agent. A request bundles
// one or more questions; the agent stays blocked until every question has been
// answered (or the bundle is rejected as a whole).
type Request struct {
	PendingID string     `json:"pending_id"`
	SessionID string     `json:"session_id"`
	TaskID    string     `json:"task_id"`
	Questions []Question `json:"questions"`         // 1-N questions, all required
	Context   string     `json:"context,omitempty"` // Optional shared context for all questions
	// Approval, when set, marks this bundle as a request_approval_kandev
	// approval request. The bundle still carries exactly one fixed question
	// so existing chat/inbox rendering treats it as a normal bundle.
	Approval *ApprovalMeta `json:"approval,omitempty"`
	// ContinuationRecovery, when set, marks this bundle as a system-authored
	// continuation-recovery request. It carries exactly one fixed question and
	// has no agent waiter; the resolver forwards the decision to the
	// orchestrator's recovery handler.
	ContinuationRecovery *ContinuationRecoveryMeta `json:"continuation_recovery,omitempty"`
	CreatedAt            time.Time                 `json:"created_at"`
}

// Answer represents the user's answer to a single question.
type Answer struct {
	QuestionID      string   `json:"question_id"`
	SelectedOptions []string `json:"selected_options,omitempty"` // Option IDs (single-choice ⇒ at most one)
	CustomText      string   `json:"custom_text,omitempty"`      // Free-text input
	// PlanCommentRefs carries the exact pending plan comments the user
	// attached to an approval decision. Only valid on an approval bundle
	// whose subject is a task plan.
	PlanCommentRefs []taskmodels.TaskPlanCommentRef `json:"plan_comment_refs,omitempty"`
}

// Response represents the user's response to a clarification request.
// On success, Answers has exactly one entry per question in the request.
// On rejection, Answers may be nil and Rejected/RejectReason describe the skip.
type Response struct {
	PendingID    string    `json:"pending_id"`
	Answers      []Answer  `json:"answers,omitempty"`
	Rejected     bool      `json:"rejected,omitempty"`
	RejectReason string    `json:"reject_reason,omitempty"` // If rejected
	RespondedAt  time.Time `json:"responded_at"`
	// Approval carries the resolver-filled outcome of an approval bundle.
	Approval *ApprovalOutcome `json:"approval,omitempty"`
}

// PendingClarification represents a clarification request waiting for a response.
type PendingClarification struct {
	Request   *Request
	done      chan struct{} // Closed when a response is submitted (broadcast to all waiters)
	resp      *Response
	mu        sync.Mutex
	resolved  bool
	cancelled bool
	// mu guards the deliveryConfirmation* and deliveryAbandoned fields except
	// deliveryConfirmationOnce. deliveryConfirmationDone exists only when a
	// callback was supplied; started and complete distinguish an in-flight
	// callback from its result.
	deliveryConfirmation         func() error
	deliveryConfirmationOnce     sync.Once
	deliveryConfirmationErr      error
	deliveryConfirmationDone     chan struct{}
	deliveryConfirmationStarted  bool
	deliveryConfirmationComplete bool
	deliveryAbandoned            bool
	CancelCh                     chan struct{} // Closed when session's turn completes (agent moved on)
	CreatedAt                    time.Time
}

// Status represents the status of a clarification request.
type Status string

const (
	StatusPending   Status = "pending"
	StatusAnswered  Status = "answered"
	StatusRejected  Status = "rejected"
	StatusExpired   Status = "expired"
	StatusCancelled Status = "cancelled"
)
