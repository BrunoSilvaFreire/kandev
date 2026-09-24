package sqlite

// Behavioral coverage for ListTaskUsageTotalGroups: the finest-grain
// (session, agent profile, agent type, model, provider) breakdown the per-agent
// Usage panel reads. See docs/specs/task-cost-ledger/spec.md.

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func usageGroupsFor(t *testing.T, repo *Repository, taskID string) []*models.TaskUsageTotalsGroup {
	t.Helper()
	groups, err := repo.ListTaskUsageTotalGroups(context.Background(), taskID)
	if err != nil {
		t.Fatalf("ListTaskUsageTotalGroups(%s): %v", taskID, err)
	}
	return groups
}

// TestListTaskUsageTotalGroups_GroupsBySessionAndModel pins the grain, the
// per-group sums/counts/timestamps, and the newest-activity-first ordering.
func TestListTaskUsageTotalGroups_GroupsBySessionAndModel(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	createUsageEventsTestTask(t, repo, "task-groups")
	createUsageEventsTestSession(t, repo, "session-a", "task-groups")
	createUsageEventsTestSession(t, repo, "session-b", "task-groups")

	base := time.Date(2026, 8, 23, 4, 0, 0, 0, time.UTC)

	modelX1 := newTestUsageEvent("evt-a1", "task-groups", "session-a")
	modelX1.AgentProfileID = "profile-1"
	modelX1.Model = "model-x"
	modelX1.Provider = "provider-1"
	modelX1.OccurredAt = base
	modelX1.CreatedAt = base
	mustCreateUsageEvent(t, repo, modelX1)

	modelX2 := newTestUsageEvent("evt-a2", "task-groups", "session-a")
	modelX2.AgentProfileID = "profile-1"
	modelX2.Model = "model-x"
	modelX2.Provider = "provider-1"
	modelX2.OccurredAt = base.Add(5 * time.Minute)
	modelX2.CreatedAt = modelX2.OccurredAt
	mustCreateUsageEvent(t, repo, modelX2)

	modelY := newTestUsageEvent("evt-a3", "task-groups", "session-a")
	modelY.AgentProfileID = "profile-1"
	modelY.Model = "model-y"
	modelY.Provider = "provider-1"
	modelY.OccurredAt = base.Add(10 * time.Minute)
	modelY.CreatedAt = modelY.OccurredAt
	mustCreateUsageEvent(t, repo, modelY)

	sessionB := newTestUsageEvent("evt-b1", "task-groups", "session-b")
	sessionB.AgentProfileID = "profile-2"
	sessionB.Model = "model-x"
	sessionB.Provider = "provider-2"
	sessionB.OccurredAt = base.Add(20 * time.Minute)
	sessionB.CreatedAt = sessionB.OccurredAt
	mustCreateUsageEvent(t, repo, sessionB)

	groups := usageGroupsFor(t, repo, "task-groups")
	if len(groups) != 3 {
		t.Fatalf("len(groups) = %d, want 3", len(groups))
	}

	// Newest first: session-b (20m), session-a/model-y (10m), session-a/model-x (5m).
	assertGroup := func(label string, group *models.TaskUsageTotalsGroup, session, profile, model, provider string, eventCount int64) {
		t.Helper()
		if group.SessionID == nil || *group.SessionID != session {
			t.Errorf("%s SessionID = %v, want %q", label, group.SessionID, session)
		}
		if group.AgentProfileID != profile || group.Model != model || group.Provider != provider {
			t.Errorf("%s identity = (%q, %q, %q), want (%q, %q, %q)",
				label, group.AgentProfileID, group.Model, group.Provider, profile, model, provider)
		}
		if group.Totals.EventCount != eventCount {
			t.Errorf("%s EventCount = %d, want %d", label, group.Totals.EventCount, eventCount)
		}
	}

	assertGroup("groups[0]", groups[0], "session-b", "profile-2", "model-x", "provider-2", 1)
	assertGroup("groups[1]", groups[1], "session-a", "profile-1", "model-y", "provider-1", 1)
	assertGroup("groups[2]", groups[2], "session-a", "profile-1", "model-x", "provider-1", 2)

	if groups[2].Totals.TokensIn != 200 {
		t.Errorf("groups[2] TokensIn = %d, want 200 (two rows of 100)", groups[2].Totals.TokensIn)
	}
	if groups[2].Totals.CostSubcents != 84 {
		t.Errorf("groups[2] CostSubcents = %d, want 84 (two rows of 42)", groups[2].Totals.CostSubcents)
	}
	if groups[2].Totals.FirstEventAt == nil || !groups[2].Totals.FirstEventAt.Equal(base) {
		t.Errorf("groups[2] FirstEventAt = %v, want %v", groups[2].Totals.FirstEventAt, base)
	}
	if groups[2].Totals.LastEventAt == nil || !groups[2].Totals.LastEventAt.Equal(modelX2.OccurredAt) {
		t.Errorf("groups[2] LastEventAt = %v, want %v", groups[2].Totals.LastEventAt, modelX2.OccurredAt)
	}

	// The groups partition the task total exactly (no race).
	taskTotals, err := repo.GetTaskUsageTotals(context.Background(), "task-groups")
	if err != nil {
		t.Fatalf("GetTaskUsageTotals: %v", err)
	}
	var sumTokensIn, sumCost, sumEvents int64
	for _, group := range groups {
		sumTokensIn += group.Totals.TokensIn
		sumCost += group.Totals.CostSubcents
		sumEvents += group.Totals.EventCount
	}
	if sumTokensIn != taskTotals.TokensIn || sumCost != taskTotals.CostSubcents || sumEvents != taskTotals.EventCount {
		t.Errorf("group sums = (%d, %d, %d), want task totals (%d, %d, %d)",
			sumTokensIn, sumCost, sumEvents, taskTotals.TokensIn, taskTotals.CostSubcents, taskTotals.EventCount)
	}
}

// TestListTaskUsageTotalGroups_NullSessionAndConfidenceFlags pins the deleted
// session group (session_id NULL) and the estimated/unpriced/incomplete flags.
func TestListTaskUsageTotalGroups_NullSessionAndConfidenceFlags(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	createUsageEventsTestTask(t, repo, "task-groups-null")

	// Direct insert: session_id NULL, zero tokens, empty identity.
	insertUsageEventRow(t, repo, "evt-null", "task-groups-null", "")

	createUsageEventsTestSession(t, repo, "session-a", "task-groups-null")

	estimated := newTestUsageEvent("evt-estimated", "task-groups-null", "session-a")
	estimated.AgentProfileID = "profile-1"
	estimated.Estimated = true
	estimated.TokensOut = nil // marks the group's output count incomplete
	mustCreateUsageEvent(t, repo, estimated)

	unpriced := newTestUsageEvent("evt-unpriced", "task-groups-null", "session-a")
	unpriced.AgentProfileID = "profile-1"
	unpriced.CostSource = "unpriced"
	mustCreateUsageEvent(t, repo, unpriced)

	groups := usageGroupsFor(t, repo, "task-groups-null")
	if len(groups) != 2 {
		t.Fatalf("len(groups) = %d, want 2", len(groups))
	}

	var nullGroup, sessionGroup *models.TaskUsageTotalsGroup
	for _, group := range groups {
		if group.SessionID == nil {
			nullGroup = group
		} else if *group.SessionID == "session-a" {
			sessionGroup = group
		}
	}
	if nullGroup == nil {
		t.Fatal("no group with a nil SessionID (the deleted-session rows)")
	}
	if nullGroup.Totals.EventCount != 1 {
		t.Errorf("null-session EventCount = %d, want 1", nullGroup.Totals.EventCount)
	}
	if !nullGroup.Totals.OutputTokensComplete {
		t.Error("null-session OutputTokensComplete = false, want true (its row has a recorded output count)")
	}

	if sessionGroup == nil {
		t.Fatal("no group for session-a")
	}
	if sessionGroup.Totals.EventCount != 2 {
		t.Errorf("session-a EventCount = %d, want 2", sessionGroup.Totals.EventCount)
	}
	if sessionGroup.Totals.EstimatedEventCount != 1 {
		t.Errorf("session-a EstimatedEventCount = %d, want 1", sessionGroup.Totals.EstimatedEventCount)
	}
	if sessionGroup.Totals.UnpricedEventCount != 1 {
		t.Errorf("session-a UnpricedEventCount = %d, want 1", sessionGroup.Totals.UnpricedEventCount)
	}
	if sessionGroup.Totals.OutputTokensComplete {
		t.Error("session-a OutputTokensComplete = true, want false (one row has a nil tokens_out)")
	}
}

// TestListTaskUsageTotalGroups_NoRows_ReturnsEmptySlice pins that a task with
// no ledger rows (or an unknown task) returns an empty, non-nil slice.
func TestListTaskUsageTotalGroups_NoRows_ReturnsEmptySlice(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	createUsageEventsTestTask(t, repo, "task-groups-empty")

	groups := usageGroupsFor(t, repo, "task-groups-empty")
	if groups == nil {
		t.Fatal("groups = nil, want an empty slice")
	}
	if len(groups) != 0 {
		t.Errorf("len(groups) = %d, want 0", len(groups))
	}
}
