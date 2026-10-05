package service

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func str(value string) *string { return &value }

func transitionID(value int64) *int64 { return &value }

// TestStepVisitSessionsGroupsOrderedAndDeduped covers two visits with two
// sessions, including a step the session re-entered: the session appears once,
// at its first arrival, and sessions stay oldest to newest.
func TestStepVisitSessionsGroupsOrderedAndDeduped(t *testing.T) {
	base := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
	routes := []*models.TaskSessionRoute{
		{DestinationWorkflowStepID: "b", DestinationSessionID: str("s2"), WorkflowStepTransitionID: transitionID(11), CreatedAt: base.Add(2 * time.Minute)},
		{DestinationWorkflowStepID: "b", DestinationSessionID: str("s1"), WorkflowStepTransitionID: transitionID(10), CreatedAt: base.Add(time.Minute)},
		{DestinationWorkflowStepID: "b", DestinationSessionID: str("s1"), WorkflowStepTransitionID: transitionID(12), CreatedAt: base.Add(3 * time.Minute)},
		{DestinationWorkflowStepID: "a", DestinationSessionID: str("s0"), CreatedAt: base},
		{DestinationWorkflowStepID: "b", DestinationSessionID: nil, CreatedAt: base.Add(4 * time.Minute)},
	}

	byStep := stepVisitSessions(routes)

	bSessions := byStep["b"]
	if len(bSessions) != 2 {
		t.Fatalf("step b sessions = %d, want 2 (deduplicated)", len(bSessions))
	}
	if bSessions[0].SessionID != "s1" || bSessions[1].SessionID != "s2" {
		t.Fatalf("step b sessions not oldest-first or deduped: %+v", bSessions)
	}
	if bSessions[0].TransitionID == nil || *bSessions[0].TransitionID != 10 {
		t.Fatalf("step b first session must keep its first transition, got %+v", bSessions[0])
	}
	if sessions := byStep["a"]; len(sessions) != 1 || sessions[0].SessionID != "s0" {
		t.Fatalf("step a sessions = %+v, want [s0]", sessions)
	}
}
