package protocol

import "strings"

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

// ApprovalMeta is the request-side metadata that marks a clarification bundle
// as an approval request. It is persisted alongside the bundle's questions so
// the resolver can render the subject's pending comments before claiming.
type ApprovalMeta struct {
	Subject          string `json:"subject"`
	DocumentKey      string `json:"document_key,omitempty"`
	Title            string `json:"title"`
	VersionAtRequest string `json:"version_at_request,omitempty"`
	PlanRevisionID   string `json:"plan_revision_id,omitempty"`
}

// ApprovalQuestion returns the fixed single question of an approval bundle.
// The option IDs are the stable decisions the resolver records.
func ApprovalQuestion(title string) Question {
	label := strings.TrimSpace(title)
	if label == "" {
		label = "Approval"
	}
	return Question{
		ID:     ApprovalQuestionID,
		Title:  truncateRunes(label, 12),
		Prompt: label,
		Options: []Option{
			{ID: ApprovalDecisionApprove, Label: "Approve", Description: "Accept the subject as-is."},
			{ID: ApprovalDecisionRevise, Label: "Revise", Description: "Send feedback or plan comments for another revision."},
			{ID: ApprovalDecisionReject, Label: "Reject", Description: "Stop without accepting the subject."},
		},
	}
}

func truncateRunes(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}
