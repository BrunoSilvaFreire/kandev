package clarification

import (
	"context"
	"errors"
	"strings"
	"testing"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
)

func approvalTestLogger(t *testing.T) *logger.Logger {
	t.Helper()
	core, _ := observer.New(zapcore.WarnLevel)
	l, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("NewFromZap: %v", err)
	}
	return l
}

type fakeApprovalReader struct {
	current      string
	currentErr   error
	markdown     string
	ids          []string
	renderErr    error
	currentCalls int
	renderCalls  int
	lastTaskID   string
	lastSubject  string
}

func (f *fakeApprovalReader) CurrentVersion(_ context.Context, taskID, subject, _ string) (string, error) {
	f.currentCalls++
	f.lastTaskID = taskID
	f.lastSubject = subject
	return f.current, f.currentErr
}

func (f *fakeApprovalReader) RenderPlanComments(_ context.Context, taskID string, refs []taskmodels.TaskPlanCommentRef) (string, []string, error) {
	f.renderCalls++
	f.lastTaskID = taskID
	if f.renderErr != nil {
		return "", nil, f.renderErr
	}
	return f.markdown, f.ids, nil
}

type fakeApprovalConsumer struct {
	calls int
	err   error
	refs  []taskmodels.TaskPlanCommentRef
}

func (f *fakeApprovalConsumer) ConsumePlanComments(_ context.Context, _ string, refs []taskmodels.TaskPlanCommentRef) error {
	f.calls++
	f.refs = refs
	return f.err
}

func approvalOption(id string) []string { return []string{id} }

func approvalOutcome(decision, feedback string, refs []taskmodels.TaskPlanCommentRef) Outcome {
	return Outcome{Answers: []Answer{{
		QuestionID:      ApprovalQuestionID,
		SelectedOptions: approvalOption(decision),
		CustomText:      feedback,
		PlanCommentRefs: refs,
	}}}
}

func TestValidateApprovalOutcomeMatrix(t *testing.T) {
	planMeta := &ApprovalMeta{Subject: ApprovalSubjectTaskPlan}
	docMeta := &ApprovalMeta{Subject: ApprovalSubjectDocument, DocumentKey: "review"}
	refs := []taskmodels.TaskPlanCommentRef{{ID: "c1", Version: 1}}

	cases := []struct {
		name          string
		meta          *ApprovalMeta
		outcome       Outcome
		subjectEdited bool
		wantErr       bool
	}{
		{name: "approve", meta: planMeta, outcome: approvalOutcome(ApprovalDecisionApprove, "", nil)},
		{name: "approve with refs rejected", meta: planMeta, outcome: approvalOutcome(ApprovalDecisionApprove, "", refs), wantErr: true},
		{name: "reject with refs rejected", meta: planMeta, outcome: approvalOutcome(ApprovalDecisionReject, "", refs), wantErr: true},
		{name: "revise empty rejected", meta: planMeta, outcome: approvalOutcome(ApprovalDecisionRevise, "", nil), wantErr: true},
		{name: "revise with feedback", meta: planMeta, outcome: approvalOutcome(ApprovalDecisionRevise, "please fix", nil)},
		{name: "revise with refs", meta: planMeta, outcome: approvalOutcome(ApprovalDecisionRevise, "", refs)},
		{name: "revise with edited subject", meta: planMeta, outcome: approvalOutcome(ApprovalDecisionRevise, "", nil), subjectEdited: true},
		{name: "document refs rejected", meta: docMeta, outcome: approvalOutcome(ApprovalDecisionRevise, "", refs), wantErr: true},
		{name: "document feedback ok", meta: docMeta, outcome: approvalOutcome(ApprovalDecisionRevise, "fix", nil)},
		{name: "unknown decision", meta: planMeta, outcome: approvalOutcome("maybe", "", nil), wantErr: true},
		{name: "no selection", meta: planMeta, outcome: Outcome{Answers: []Answer{{QuestionID: ApprovalQuestionID, CustomText: "x"}}}, wantErr: true},
		{name: "empty ref id", meta: planMeta, outcome: approvalOutcome(ApprovalDecisionRevise, "", []taskmodels.TaskPlanCommentRef{{ID: "", Version: 1}}), wantErr: true},
		{name: "rejected outcome maps to reject", meta: planMeta, outcome: Outcome{Rejected: true, RejectReason: "no"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateApprovalOutcome(tc.meta, tc.outcome, tc.subjectEdited)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateApprovalBundleShape(t *testing.T) {
	meta := &ApprovalMeta{Subject: ApprovalSubjectTaskPlan}
	q := ApprovalQuestion("Plan revision 2")
	if err := validateApprovalBundle([]Question{q}, meta); err != nil {
		t.Fatalf("valid bundle rejected: %v", err)
	}
	if err := validateApprovalBundle([]Question{q, q}, meta); err == nil {
		t.Fatalf("expected two-question bundle to be rejected")
	}
	bad := q
	bad.Options = bad.Options[:1]
	if err := validateApprovalBundle([]Question{bad}, meta); err == nil {
		t.Fatalf("expected missing option to be rejected")
	}
	docMeta := &ApprovalMeta{Subject: ApprovalSubjectDocument}
	if err := validateApprovalBundle([]Question{q}, docMeta); err == nil {
		t.Fatalf("expected document subject without key to be rejected")
	}
	if err := validateApprovalBundle([]Question{q}, &ApprovalMeta{Subject: "nope"}); err == nil {
		t.Fatalf("expected unknown subject to be rejected")
	}
}

func TestApprovalMetaFromMessages(t *testing.T) {
	msgs := []*taskmodels.Message{{Metadata: map[string]any{
		"approval": map[string]any{"subject": "task_plan", "title": "Plan"},
	}}}
	meta := approvalMetaFromMessages(msgs)
	if meta == nil || meta.Subject != ApprovalSubjectTaskPlan || meta.Title != "Plan" {
		t.Fatalf("unexpected meta: %+v", meta)
	}
	if approvalMetaFromMessages([]*taskmodels.Message{{Metadata: map[string]any{}}}) != nil {
		t.Fatalf("expected nil for a non-approval bundle")
	}
	if approvalMetaFromMessages(nil) != nil {
		t.Fatalf("expected nil for no messages")
	}
}

func TestBuildApprovalOutcomeSubjectEdited(t *testing.T) {
	reader := &fakeApprovalReader{current: "v2"}
	r := &Resolver{approvalReader: reader, logger: approvalTestLogger(t)}
	meta := &ApprovalMeta{Subject: ApprovalSubjectTaskPlan, VersionAtRequest: "v1"}
	out, err := r.buildApprovalOutcome(context.Background(), "task-1", meta, approvalOutcome(ApprovalDecisionApprove, "", nil))
	if err != nil {
		t.Fatalf("buildApprovalOutcome: %v", err)
	}
	if !out.SubjectEdited || out.CurrentVersion != "v2" {
		t.Fatalf("expected edited=true current=v2, got %+v", out)
	}

	reader.current = "v1"
	out, err = r.buildApprovalOutcome(context.Background(), "task-1", meta, approvalOutcome(ApprovalDecisionApprove, "", nil))
	if err != nil {
		t.Fatalf("buildApprovalOutcome: %v", err)
	}
	if out.SubjectEdited {
		t.Fatalf("expected edited=false for unchanged version")
	}

	// Empty version at request time can never be proven edited.
	reader.current = "v9"
	out, err = r.buildApprovalOutcome(context.Background(), "task-1", &ApprovalMeta{Subject: ApprovalSubjectTaskPlan}, approvalOutcome(ApprovalDecisionApprove, "", nil))
	if err != nil {
		t.Fatalf("buildApprovalOutcome: %v", err)
	}
	if out.SubjectEdited {
		t.Fatalf("expected edited=false when no request version was recorded")
	}
}

func TestBuildApprovalOutcomeRendersCommentsAndWrapsStaleAsValidation(t *testing.T) {
	refs := []taskmodels.TaskPlanCommentRef{{ID: "c1", Version: 3}}
	reader := &fakeApprovalReader{markdown: "### Plan Comments\n\n> fix\n", ids: []string{"c1"}}
	r := &Resolver{approvalReader: reader, logger: approvalTestLogger(t)}
	out, err := r.buildApprovalOutcome(context.Background(), "task-1", &ApprovalMeta{Subject: ApprovalSubjectTaskPlan}, approvalOutcome(ApprovalDecisionRevise, "", refs))
	if err != nil {
		t.Fatalf("buildApprovalOutcome: %v", err)
	}
	if !strings.Contains(out.PlanComments, "### Plan Comments") || len(out.CommentIDs) != 1 {
		t.Fatalf("unexpected rendered outcome: %+v", out)
	}

	reader.renderErr = errors.New("stale")
	_, err = r.buildApprovalOutcome(context.Background(), "task-1", &ApprovalMeta{Subject: ApprovalSubjectTaskPlan}, approvalOutcome(ApprovalDecisionRevise, "", refs))
	if err == nil || !IsValidationError(err) {
		t.Fatalf("expected validation error for stale refs, got %v", err)
	}
}

func TestConsumeApprovalCommentsBestEffort(t *testing.T) {
	consumer := &fakeApprovalConsumer{err: errors.New("boom")}
	r := &Resolver{approvalConsumer: consumer, logger: approvalTestLogger(t)}
	outcome := approvalOutcome(ApprovalDecisionRevise, "", []taskmodels.TaskPlanCommentRef{{ID: "c1", Version: 1}})
	// A consumption failure must not panic or propagate; the answer is already delivered.
	r.consumeApprovalComments(context.Background(), "task-1", "pending-1", outcome, &ApprovalOutcome{Decision: ApprovalDecisionRevise, CommentIDs: []string{"c1"}})
	if consumer.calls != 1 {
		t.Fatalf("expected consumer called once, got %d", consumer.calls)
	}
	// Non-revise decisions consume nothing.
	consumer.calls = 0
	r.consumeApprovalComments(context.Background(), "task-1", "pending-1", approvalOutcome(ApprovalDecisionApprove, "", nil), &ApprovalOutcome{Decision: ApprovalDecisionApprove})
	if consumer.calls != 0 {
		t.Fatalf("approve must not consume comments")
	}
}

func TestFormatApprovalOutcome(t *testing.T) {
	text := FormatApprovalOutcome(&ApprovalOutcome{
		Decision:       ApprovalDecisionRevise,
		Feedback:       "tighten scope",
		PlanComments:   "### Plan Comments\n\n> do it\n",
		SubjectEdited:  true,
		CurrentVersion: "v7",
	})
	for _, want := range []string{"Approval decision: revise", "tighten scope", "Subject edited by user: yes (current version v7)", "### Plan Comments"} {
		if !strings.Contains(text, want) {
			t.Fatalf("FormatApprovalOutcome missing %q in:\n%s", want, text)
		}
	}
	if FormatApprovalOutcome(nil) != "" {
		t.Fatalf("nil approval must render empty")
	}
}

func TestSerializeResponseCarriesApproval(t *testing.T) {
	resp := &Response{
		PendingID: "p1",
		Answers: []Answer{{
			QuestionID:      ApprovalQuestionID,
			SelectedOptions: []string{ApprovalDecisionRevise},
			PlanCommentRefs: []taskmodels.TaskPlanCommentRef{{ID: "c1", Version: 2}},
		}},
		Approval: &ApprovalOutcome{Decision: ApprovalDecisionRevise, CommentIDs: []string{"c1"}, CurrentVersion: "v3"},
	}
	data, err := SerializeResponse(resp)
	if err != nil {
		t.Fatalf("SerializeResponse: %v", err)
	}
	back, err := DeserializeResponse(data)
	if err != nil {
		t.Fatalf("DeserializeResponse: %v", err)
	}
	if back.Approval == nil || back.Approval.Decision != ApprovalDecisionRevise || back.Approval.CurrentVersion != "v3" {
		t.Fatalf("approval lost in round trip: %+v", back.Approval)
	}
	if len(back.Answers) != 1 || len(back.Answers[0].PlanCommentRefs) != 1 || back.Answers[0].PlanCommentRefs[0].ID != "c1" {
		t.Fatalf("plan comment refs lost in round trip: %+v", back.Answers)
	}
}

func TestApprovalOutcomeFromStoredMetadata(t *testing.T) {
	meta := map[string]any{
		"response": map[string]any{
			"selected_options": []interface{}{ApprovalDecisionRevise},
			"custom_text":      "fix",
			"approval": map[string]any{
				"decision":        ApprovalDecisionRevise,
				"comment_ids":     []interface{}{"c1"},
				"subject_edited":  true,
				"current_version": "v4",
			},
		},
	}
	out := approvalOutcomeFromMetadata(meta)
	if out == nil || out.Decision != ApprovalDecisionRevise || !out.SubjectEdited || len(out.CommentIDs) != 1 {
		t.Fatalf("unexpected reconstructed approval: %+v", out)
	}
	answer, ok := answerFromMessageMetadata("m1", meta, nil)
	if !ok || answer.CustomText != "fix" {
		t.Fatalf("answer reconstruction failed: %+v ok=%v", answer, ok)
	}
}
