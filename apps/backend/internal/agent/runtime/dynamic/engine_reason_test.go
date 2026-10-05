package dynamic

import (
	"context"
	"testing"
)

func TestSelectContextUsesCandidateScheduleReason(t *testing.T) {
	engine := NewEngine()
	profile := Profile{ID: "dyn", Version: 1, Candidates: []Candidate{
		{ID: "preferred", Enabled: true, Reason: "preferred_tag_match"},
	}}
	decision, err := engine.SelectContext(context.Background(), "s1", profile, 0, "")
	if err != nil {
		t.Fatalf("SelectContext: %v", err)
	}
	if decision.Reason != "preferred_tag_match" {
		t.Fatalf("reason = %q, want preferred_tag_match", decision.Reason)
	}
}

func TestSelectContextExplicitReasonWinsOverCandidateReason(t *testing.T) {
	engine := NewEngine()
	profile := Profile{ID: "dyn", Version: 1, Candidates: []Candidate{
		{ID: "candidate", Enabled: true, Reason: "preferred_tag_match"},
	}}
	decision, err := engine.SelectContextWithReason(context.Background(), "s2", profile, 0, "", "manual_skip")
	if err != nil {
		t.Fatalf("SelectContextWithReason: %v", err)
	}
	if decision.Reason != "manual_skip" {
		t.Fatalf("reason = %q, want manual_skip", decision.Reason)
	}
}

func TestSelectContextDefaultsToCandidateOrderWithoutReason(t *testing.T) {
	engine := NewEngine()
	profile := Profile{ID: "dyn", Version: 1, Candidates: []Candidate{{ID: "candidate", Enabled: true}}}
	decision, err := engine.SelectContext(context.Background(), "s3", profile, 0, "")
	if err != nil {
		t.Fatalf("SelectContext: %v", err)
	}
	if decision.Reason != "candidate_order" {
		t.Fatalf("reason = %q, want candidate_order", decision.Reason)
	}
}
