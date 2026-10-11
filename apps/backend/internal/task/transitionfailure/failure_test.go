package transitionfailure

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowmove "github.com/kandev/kandev/internal/workflow/move"
)

type codedErr struct{}

func (codedErr) Error() string                 { return "invalid workflow change" }
func (codedErr) TransitionFailureCode() string { return "workflow_change_profile_missing" }

func TestClassifyKeepsCodesAndGivesEveryFailureAReasonAndAction(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code string
		rule string
	}{
		{"archived", Rule(RuleArchived, "archived tasks cannot be moved"), CodeArchived, RuleArchived},
		{"active session", fmt.Errorf("move: %w", RuleObserved(RuleActiveSession, "task has an active session (RUNNING)", "session RUNNING")), CodeActiveSession, RuleActiveSession},
		{"workspace", Rule(RuleDifferentWorkspace, "x"), CodeDifferentWorkspace, RuleDifferentWorkspace},
		{"foreign step", Rule(RuleStepNotInWorkflow, "x"), CodeWorkflowStep, RuleStepNotInWorkflow},
		{"unknown transition", Rule(RuleUnknownTransition, "x"), CodeTransitionUnknown, RuleUnknownTransition},
		{"wip", fmt.Errorf("%w: 3/3", wfmodels.ErrWIPLimitExceeded), CodeWIPLimit, RuleWIPLimit},
		{"pending", workflowmove.ErrMoveConflict, CodePending, RulePendingMove},
		{"workflow change conflict", repoerrors.ErrWorkflowChangeConflict, CodeWorkflowChangeConflict, RuleTaskChanged},
		{"stale version", repoerrors.ErrTaskVersionConflict, CodeStale, RuleTaskChanged},
		{"entry options", workflowmove.ErrEntryOptionsUnsupported, CodeInvalid, RuleEntryOptions},
		{"entry target", workflowmove.ErrEntryTargetUnavailable, CodeInvalid, RuleEntryTarget},
		{"criteria conflict", repoerrors.ErrTaskCompletionCriteriaConflict, CodeCriteriaConflict, RuleCriteriaChanged},
		{"human confirmation", repoerrors.ErrTaskCompletionHumanConfirmationRequired, CodeHumanConfirmationRequired, RuleHumanConfirmation},
		{"coded validation", codedErr{}, "workflow_change_profile_missing", RuleInvalidRequest},
		{"not found", repoerrors.ErrTaskNotFound, CodeNotFound, RuleTargetNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Classify(tc.err, Input{})
			if got.Code != tc.code || got.Actionability != ActionabilityUser {
				t.Fatalf("code/actionability = %q/%q, want %q/user", got.Code, got.Actionability, tc.code)
			}
			if len(got.Reasons) != 1 || got.Reasons[0].Rule != tc.rule {
				t.Fatalf("reasons = %+v, want one %q", got.Reasons, tc.rule)
			}
			if got.Message == "" || got.Reasons[0].Action == "" || got.Remediation == "" || got.IncidentID != "" {
				t.Fatalf("incomplete envelope: %+v", got)
			}
		})
	}
}

func TestClassifyUnknownCauseIsASafeSystemFailure(t *testing.T) {
	got := Classify(errors.New(`pq: relation "tasks" deadlock at /srv/secret/path`), Input{})
	if got.Code != CodeInternal || got.Actionability != ActionabilitySystem || got.IncidentID == "" {
		t.Fatalf("unexpected envelope: %+v", got)
	}
	raw, _ := json.Marshal(got.Details())
	if strings.Contains(string(raw), "pq:") || strings.Contains(string(raw), "secret") {
		t.Fatalf("raw error text leaked: %s", raw)
	}
}

func TestClassifyRedactsInaccessibleObjects(t *testing.T) {
	got := Classify(errors.New("forbidden: workspace acme-internal"), Input{NotVisible: true})
	raw, _ := json.Marshal(got)
	if got.Code != CodeNotFound || strings.Contains(string(raw), "acme") {
		t.Fatalf("unexpected envelope: %s", raw)
	}
}

func gate(stale bool) *models.TaskCompletionGateSnapshot {
	snapshot := &models.TaskCompletionGateSnapshot{
		Revision: 6, PlanRevisionID: "rev-9", PlanWriteVersion: "w1", CurrentPlanWriteVersion: "w1", Blocked: true,
		Criteria: []models.TaskCompletionCriterion{
			{ID: "I1", Description: "Workflow updates in place", CriterionRevision: 1},
			{ID: "I2", Description: "Visits survive restart", CriterionRevision: 1, VerifiedRevision: 1},
			{ID: "I3", Description: "Inbox is truthful", CriterionRevision: 2, VerifiedRevision: 1},
		},
		Blockers: []models.TaskCompletionBlocker{
			{CriterionID: "I1", Reason: "unverified"},
			{CriterionID: "I2", Reason: "evidence_stale"},
			{CriterionID: "I3", Reason: "unverified"},
		},
	}
	if stale {
		snapshot.PlanStale = true
		snapshot.CurrentPlanWriteVersion = "w2"
		snapshot.Blockers = append(snapshot.Blockers, models.TaskCompletionBlocker{
			CriterionID: "plan", Reason: "plan_revision_stale: task plan was modified after criteria enrollment",
		})
	}
	return snapshot
}

func TestGateBlockedNamesEveryUnmetCriterionAndVerification(t *testing.T) {
	got := Classify(fmt.Errorf("move: %w", repoerrors.ErrTaskCompletionGateBlocked), Input{Gate: gate(false)})
	if got.Code != CodeGateBlocked || got.Remediation != remediationGateUnverified {
		t.Fatalf("unexpected envelope: %+v", got)
	}
	if got.Gate == nil || got.Gate.Revision != 6 || got.Gate.EnrolledPlanRevisionID != "rev-9" || got.Gate.PlanStale {
		t.Fatalf("gate identities = %+v", got.Gate)
	}
	want := []struct{ id, rule, description string }{
		{"I1", RuleCriterionUnverified, "Workflow updates in place"},
		{"I2", RuleEvidenceStale, "Visits survive restart"},
		{"I3", RuleCriterionUnverified, "Inbox is truthful"},
	}
	if len(got.Reasons) != len(want) {
		t.Fatalf("reasons = %+v", got.Reasons)
	}
	for i, w := range want {
		r := got.Reasons[i]
		if r.CriterionID != w.id || r.Rule != w.rule || r.Description != w.description || r.Action == "" {
			t.Fatalf("reason %d = %+v, want %+v", i, r, w)
		}
	}
}

func TestGateBlockedOnStalePlanAsksForApprovalAndReEnrollmentFirst(t *testing.T) {
	got := Classify(repoerrors.ErrTaskCompletionGateBlocked, Input{Gate: gate(true)})
	if got.Remediation != remediationGatePlanStale || len(got.Reasons) != 4 {
		t.Fatalf("unexpected envelope: %+v", got)
	}
	first := got.Reasons[0]
	if first.Rule != RulePlanStale || !strings.Contains(first.Observed, "w2") || !strings.Contains(first.Required, "w1") ||
		first.CTA == nil || first.CTA.Kind != "open_plan" {
		t.Fatalf("stale reason = %+v", first)
	}
	if got.Gate.CurrentPlanWriteVersion != "w2" || got.Gate.EnrolledPlanWriteVersion != "w1" || !got.Gate.PlanStale {
		t.Fatalf("gate identities = %+v", got.Gate)
	}
}

func TestGateBlockedWithoutSnapshotStillExplains(t *testing.T) {
	got := Classify(repoerrors.ErrTaskCompletionGateBlocked, Input{})
	if got.Code != CodeGateBlocked || len(got.Reasons) != 1 || got.Reasons[0].Action == "" || got.Gate != nil {
		t.Fatalf("unexpected envelope: %+v", got)
	}
}

func TestDetailsKeepsLegacyCodeKeys(t *testing.T) {
	details := Classify(repoerrors.ErrTaskCompletionGateBlocked, Input{Gate: gate(false)}).Details()
	if details["code"] != CodeGateBlocked || details["error_code"] != CodeGateBlocked || details["gate"] == nil {
		t.Fatalf("details = %+v", details)
	}
}
