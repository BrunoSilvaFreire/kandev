package clarification

import (
	"context"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/clarification/protocol"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

// ApprovalSubjectReader reads the approval subject's current version and
// renders the exact pending plan comments a revise answer references. It is
// implemented in task/service and injected into the Resolver; the resolver
// authorizes through the bundle's task, never a client-supplied id.
type ApprovalSubjectReader interface {
	// CurrentVersion returns the subject's current optimistic-concurrency
	// version. For a task plan this is the plan write version; for a document
	// it is the document's current version.
	CurrentVersion(ctx context.Context, taskID, subject, documentKey string) (string, error)
	// RenderPlanComments validates refs against the task's current pending
	// comments and returns the canonical Markdown block plus the resolved
	// comment ids in the same order.
	RenderPlanComments(ctx context.Context, taskID string, refs []taskmodels.TaskPlanCommentRef) (string, []string, error)
}

// ApprovalCommentConsumer consumes the plan comments a revise answer carried.
// It is best-effort: a failure leaves the comments pending, risking duplicate
// feedback but never losing it.
type ApprovalCommentConsumer interface {
	ConsumePlanComments(ctx context.Context, taskID string, refs []taskmodels.TaskPlanCommentRef) error
}

// ApprovalQuestion returns the fixed single question of an approval bundle.
// The option IDs are the stable decisions the resolver records.
func ApprovalQuestion(title string) Question {
	return protocol.ApprovalQuestion(title)
}

// IsApprovalBundle reports whether msgs belong to an approval request.
func IsApprovalBundle(msgs []*taskmodels.Message) bool {
	return approvalMetaFromMessages(msgs) != nil
}

// approvalMetaFromMessages extracts the approval metadata persisted on the
// bundle's messages. It returns nil for a normal clarification bundle.
func approvalMetaFromMessages(msgs []*taskmodels.Message) *ApprovalMeta {
	for _, m := range msgs {
		if m == nil || m.Metadata == nil {
			continue
		}
		raw, ok := m.Metadata["approval"]
		if !ok {
			continue
		}
		if meta := decodeApprovalMeta(raw); meta != nil {
			return meta
		}
	}
	return nil
}

func decodeApprovalMeta(raw any) *ApprovalMeta {
	switch v := raw.(type) {
	case *ApprovalMeta:
		return v
	case ApprovalMeta:
		return &v
	case map[string]any:
		meta := &ApprovalMeta{}
		meta.Subject, _ = v["subject"].(string)
		meta.DocumentKey, _ = v["document_key"].(string)
		meta.Title, _ = v["title"].(string)
		meta.VersionAtRequest, _ = v["version_at_request"].(string)
		if meta.Subject == "" {
			return nil
		}
		return meta
	default:
		return nil
	}
}

// validateApprovalBundle enforces the fixed shape of an approval request:
// exactly one question whose option IDs are the three stable decisions.
func validateApprovalBundle(questions []Question, meta *ApprovalMeta) error {
	if meta == nil {
		return nil
	}
	switch meta.Subject {
	case ApprovalSubjectTaskPlan, ApprovalSubjectDocument:
	default:
		return newValidationError("approval subject %q is not supported", meta.Subject)
	}
	if meta.Subject == ApprovalSubjectDocument && strings.TrimSpace(meta.DocumentKey) == "" {
		return newValidationError("approval subject %q requires document_key", meta.Subject)
	}
	if len(questions) != 1 {
		return newValidationError("approval bundle must have exactly one question")
	}
	q := questions[0]
	if q.ID != ApprovalQuestionID {
		return newValidationError("approval bundle question id must be %q", ApprovalQuestionID)
	}
	valid := map[string]bool{
		ApprovalDecisionApprove: false,
		ApprovalDecisionRevise:  false,
		ApprovalDecisionReject:  false,
	}
	for _, opt := range q.Options {
		if _, ok := valid[opt.ID]; ok {
			valid[opt.ID] = true
		}
	}
	for id, seen := range valid {
		if !seen {
			return newValidationError("approval bundle is missing option %q", id)
		}
	}
	return nil
}

// approvalDecision extracts the recorded decision, feedback, and plan-comment
// refs from a validated outcome. A rejected outcome maps to reject.
func approvalDecision(outcome Outcome) (decision, feedback string, refs []taskmodels.TaskPlanCommentRef, err error) {
	if outcome.Rejected {
		return ApprovalDecisionReject, strings.TrimSpace(outcome.RejectReason), nil, nil
	}
	if len(outcome.Answers) != 1 {
		return "", "", nil, newValidationError("approval bundle expects exactly one answer")
	}
	answer := outcome.Answers[0]
	feedback = strings.TrimSpace(answer.CustomText)
	refs = answer.PlanCommentRefs
	selected := answer.SelectedOptions
	if len(selected) != 1 {
		return "", "", nil, newValidationError("approval answer must select exactly one decision")
	}
	switch selected[0] {
	case ApprovalDecisionApprove, ApprovalDecisionRevise, ApprovalDecisionReject:
		return selected[0], feedback, refs, nil
	default:
		return "", "", nil, newValidationError("approval answer references unknown decision %q", selected[0])
	}
}

// validateApprovalOutcome applies the approval-specific rules on top of the
// generic outcome validation.
func validateApprovalOutcome(meta *ApprovalMeta, outcome Outcome, subjectEdited bool) error {
	decision, feedback, refs, err := approvalDecision(outcome)
	if err != nil {
		return err
	}
	if len(refs) > 0 {
		if meta.Subject != ApprovalSubjectTaskPlan {
			return newValidationError("plan_comment_refs are only allowed for a task_plan approval")
		}
		for _, ref := range refs {
			if strings.TrimSpace(ref.ID) == "" {
				return newValidationError("plan_comment_refs contains an empty comment id")
			}
		}
	}
	switch decision {
	case ApprovalDecisionApprove, ApprovalDecisionReject:
		if len(refs) > 0 {
			return newValidationError("%s must not carry plan_comment_refs", decision)
		}
	case ApprovalDecisionRevise:
		if feedback == "" && len(refs) == 0 && !subjectEdited {
			return newValidationError("revise requires feedback, plan comments, or an edited subject")
		}
	}
	return nil
}

// buildApprovalOutcome fills the resolver-side ApprovalOutcome for a validated
// outcome, reading the subject version and rendering pending plan comments.
func (r *Resolver) buildApprovalOutcome(
	ctx context.Context,
	taskID string,
	meta *ApprovalMeta,
	outcome Outcome,
) (*ApprovalOutcome, error) {
	decision, feedback, refs, err := approvalDecision(outcome)
	if err != nil {
		return nil, err
	}
	if len(refs) > 0 && (decision != ApprovalDecisionRevise || meta.Subject != ApprovalSubjectTaskPlan) {
		return nil, newValidationError("plan_comment_refs are only allowed for a revise on a task plan")
	}
	current, err := r.approvalCurrentVersion(ctx, taskID, meta)
	if err != nil {
		return nil, err
	}
	edited := meta.VersionAtRequest != "" && current != "" && current != meta.VersionAtRequest
	out := &ApprovalOutcome{
		Decision:       decision,
		Feedback:       feedback,
		SubjectEdited:  edited,
		CurrentVersion: current,
	}
	if len(refs) > 0 {
		if r.approvalReader == nil {
			return nil, newValidationError("approval subject reader is unavailable")
		}
		markdown, ids, renderErr := r.approvalReader.RenderPlanComments(ctx, taskID, refs)
		if renderErr != nil {
			return nil, newValidationError("approval plan comments are no longer pending: %v", renderErr)
		}
		out.PlanComments = markdown
		out.CommentIDs = ids
	}
	return out, nil
}

// FillApprovalVersion records the subject's current version on meta at request
// time, so a later resolution can detect that the user edited the subject. It
// is best-effort: on failure the version stays empty and subject_edited is
// reported as false.
func (r *Resolver) FillApprovalVersion(ctx context.Context, taskID string, meta *ApprovalMeta) {
	if r == nil || meta == nil || r.approvalReader == nil {
		return
	}
	version, err := r.approvalReader.CurrentVersion(ctx, taskID, meta.Subject, meta.DocumentKey)
	if err != nil {
		r.logger.Warn("failed to record approval subject version at request time",
			zap.String("task_id", taskID),
			zap.String("subject", meta.Subject),
			zap.Error(err))
		return
	}
	meta.VersionAtRequest = version
}

func (r *Resolver) approvalCurrentVersion(ctx context.Context, taskID string, meta *ApprovalMeta) (string, error) {
	if r.approvalReader == nil {
		return "", nil
	}
	version, err := r.approvalReader.CurrentVersion(ctx, taskID, meta.Subject, meta.DocumentKey)
	if err != nil {
		return "", fmt.Errorf("read approval subject version: %w", err)
	}
	return version, nil
}

// FormatApprovalOutcome renders the human-readable approval block appended to
// the fallback resume prompt and the primary-answer summary. It returns "" for
// a non-approval response.
func FormatApprovalOutcome(a *ApprovalOutcome) string {
	if a == nil {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Approval decision: %s\n", a.Decision)
	if a.Feedback != "" {
		fmt.Fprintf(&b, "Feedback: %s\n", a.Feedback)
	}
	edited := "no"
	if a.SubjectEdited {
		edited = "yes"
	}
	fmt.Fprintf(&b, "Subject edited by user: %s (current version %s)\n", edited, a.CurrentVersion)
	if a.PlanComments != "" {
		b.WriteString(a.PlanComments)
	}
	return strings.TrimRight(b.String(), "\n")
}

// consumeApprovalComments consumes the plan comments a revise answer carried.
// Best-effort: a failure leaves the comments pending (risking duplicate
// feedback) but never blocks or fails the already-delivered answer.
func (r *Resolver) consumeApprovalComments(
	ctx context.Context,
	taskID, pendingID string,
	outcome Outcome,
	approval *ApprovalOutcome,
) {
	if approval == nil || r.approvalConsumer == nil || len(approval.CommentIDs) == 0 {
		return
	}
	refs := approvalRefs(outcome)
	if len(refs) == 0 {
		return
	}
	consumeCtx, cancel := clarificationPersistenceContext(ctx)
	defer cancel()
	if err := r.approvalConsumer.ConsumePlanComments(consumeCtx, taskID, refs); err != nil {
		r.logger.Warn("failed to consume approval plan comments; leaving them pending",
			zap.String("pending_id", pendingID),
			zap.String("task_id", taskID),
			zap.Int("comment_count", len(refs)),
			zap.Error(err))
	}
}

// approvalRefs returns the plan-comment refs carried by an approval outcome's
// single answer. Empty for a non-revise decision.
func approvalRefs(outcome Outcome) []taskmodels.TaskPlanCommentRef {
	if outcome.Rejected || len(outcome.Answers) != 1 {
		return nil
	}
	return outcome.Answers[0].PlanCommentRefs
}

// appendApprovalBlock appends the rendered approval block to an answer summary.
// A nil approval returns the summary unchanged.
func appendApprovalBlock(summary string, approval *ApprovalOutcome) string {
	block := FormatApprovalOutcome(approval)
	if block == "" {
		return summary
	}
	if summary == "" {
		return block
	}
	return summary + "\n\n" + block
}
