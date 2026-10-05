package orchestrator

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// TestRouteDecisionConstructorsCarryDetail pins the closed-key decision detail
// each enriched constructor records. Detail-less decisions stay empty so legacy
// rows and new rows share one NULL representation.
func TestRouteDecisionConstructorsCarryDetail(t *testing.T) {
	cases := []struct {
		name     string
		decision workflowRouteDecision
		want     map[string]string
	}{
		{
			name:     "reused_existing",
			decision: decisionReusedExisting("session-b", "WAITING_FOR_INPUT"),
			want:     map[string]string{"candidate_session_id": "session-b", "candidate_state": "WAITING_FOR_INPUT"},
		},
		{
			name:     "selected_candidate_terminal",
			decision: decisionSelectedCandidateTerminal("session-c", "COMPLETED"),
			want:     map[string]string{"candidate_session_id": "session-c", "candidate_state": "COMPLETED"},
		},
		{
			name:     "forced_new_policy",
			decision: decisionForcedNewPolicy("new"),
			want:     map[string]string{"start_policy": "new"},
		},
		{
			name:     "exact_model_incompatibility",
			decision: decisionExactModelIncompatibility("gpt-terra", "gpt-luna"),
			want:     map[string]string{"required_model": "gpt-terra", "candidate_model": "gpt-luna"},
		},
		{
			name:     "step_primary",
			decision: decisionStepPrimary("session-p", "warm"),
			want:     map[string]string{"candidate_session_id": "session-p", "fitness": "warm"},
		},
		{
			name:     "unavailable_handoff",
			decision: decisionUnavailableHandoff("session-u", "unavailable"),
			want:     map[string]string{"source_session_id": "session-u", "fitness": "unavailable"},
		},
		{
			name:     "reused_current_session",
			decision: decisionReusedCurrentSession(),
			want:     map[string]string{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if len(tc.want) == 0 {
				require.Empty(t, tc.decision.Detail)
				return
			}
			var detail map[string]string
			require.NoError(t, json.Unmarshal([]byte(tc.decision.Detail), &detail))
			require.Equal(t, tc.want, detail)
		})
	}
}

// TestRecordWorkflowRouteDecisionPersistsDetail proves the recorder stores the
// decision's detail on the durable route row.
func TestRecordWorkflowRouteDecisionPersistsDetail(t *testing.T) {
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyReuse, models.WorkflowProfileSessionEndPolicyPark)
	decision := decisionReusedExisting("session-b", "WAITING_FOR_INPUT")
	step := newProfileSwitchStep("step-b", 1, models.WorkflowProfileSessionStartPolicyReuse)

	fixture.svc.recordWorkflowRouteDecision(context.Background(), "t1", fixture.current, fixture.current, step, decision, "reuse", "park")

	rows := routeLedgerRows(t, fixture.repo, "t1")
	require.Len(t, rows, 1)
	require.NotNil(t, rows[0].DecisionDetail)
	var detail map[string]string
	require.NoError(t, json.Unmarshal([]byte(*rows[0].DecisionDetail), &detail))
	require.Equal(t, "session-b", detail["candidate_session_id"])
	require.Equal(t, "WAITING_FOR_INPUT", detail["candidate_state"])
}
