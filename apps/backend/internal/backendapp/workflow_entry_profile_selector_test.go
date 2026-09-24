package backendapp

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	agentregistry "github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/selection"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	agentusage "github.com/kandev/kandev/internal/agent/usage"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

func TestStaticFallbackUsesConfiguredFallback(t *testing.T) {
	got, err := staticFallback("profile-fallback", nil)
	if err != nil || got != "profile-fallback" {
		t.Fatalf("got (%q, %v), want profile-fallback", got, err)
	}
}

func TestStaticFallbackRejectsExhaustedCandidate(t *testing.T) {
	_, err := staticFallback("profile-a", []selection.Candidate{{ProfileID: "profile-a"}})
	if !errors.Is(err, taskservice.ErrNoEligibleEntryProfile) {
		t.Fatalf("err = %v, want ErrNoEligibleEntryProfile", err)
	}
}

func TestStaticFallbackMissingFallbackErrors(t *testing.T) {
	_, err := staticFallback("", nil)
	if !errors.Is(err, taskservice.ErrNoEligibleEntryProfile) {
		t.Fatalf("err = %v, want ErrNoEligibleEntryProfile", err)
	}
}

func TestEligibleConcreteProfileExcludesOfficeAndDynamic(t *testing.T) {
	if eligibleConcreteProfile(nil) {
		t.Fatal("nil profile must be ineligible")
	}
	if eligibleConcreteProfile(&settingsmodels.AgentProfile{Enabled: true, WorkspaceID: "ws-1"}) {
		t.Fatal("workspace-scoped profile must be ineligible")
	}
	if eligibleConcreteProfile(&settingsmodels.AgentProfile{Enabled: true, AgentID: "dynamic"}) {
		t.Fatal("dynamic profile must be ineligible")
	}
	if !eligibleConcreteProfile(&settingsmodels.AgentProfile{Enabled: true, AgentID: "claude-acp"}) {
		t.Fatal("global enabled concrete profile must be eligible")
	}
}

// stubSelectorStore embeds the full settings repository but implements only
// the reads candidate listing and compatibility validation use.
type stubSelectorStore struct {
	settingsstore.Repository
	agents  []*settingsmodels.Agent
	byAgent map[string][]*settingsmodels.AgentProfile
}

func (s stubSelectorStore) ListAgents(context.Context) ([]*settingsmodels.Agent, error) {
	return s.agents, nil
}

func (s stubSelectorStore) ListAgentProfiles(_ context.Context, agentID string) ([]*settingsmodels.AgentProfile, error) {
	return s.byAgent[agentID], nil
}

func (s stubSelectorStore) GetAgent(_ context.Context, id string) (*settingsmodels.Agent, error) {
	for _, agent := range s.agents {
		if agent.ID == id {
			return agent, nil
		}
	}
	return nil, nil
}

func TestSelectEntryProfileExcludesExecutorIncompatibleCandidate(t *testing.T) {
	log := testLogger(t)
	reg := agentregistry.NewRegistry(log)
	if err := reg.Register(agents.NewClaudeACP()); err != nil {
		t.Fatalf("register agent: %v", err)
	}
	store := stubSelectorStore{
		agents: []*settingsmodels.Agent{{ID: "a1", Name: "claude-acp"}},
		byAgent: map[string][]*settingsmodels.AgentProfile{
			"a1": {
				// Lower ID would win a tie, but its agent family cannot execute.
				{ID: "aaa", AgentID: "ghost", Enabled: true, Tags: []string{"review"}},
				{ID: "zzz", AgentID: "a1", Enabled: true, Tags: []string{"review"}},
			},
		},
	}
	selector := &workflowEntryProfileSelector{
		settingsStore: store,
		agentRegistry: reg,
		strategy:      &selection.QuotaStrategy{Provider: unknownUsage{}},
		validator:     taskAgentExecutorCompatibilityValidator{profiles: store, agentRegistry: reg},
		logger:        log,
	}
	executor := &taskmodels.Executor{Type: taskmodels.ExecutorTypeLocal}

	got, err := selector.SelectEntryProfile(context.Background(), "step", []string{"review"}, "", executor, nil)
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if got != "zzz" {
		t.Fatalf("selected %q, want the executor-compatible zzz", got)
	}

	// Without executor context the filter is permissive, proving the exclusion
	// above came from compatibility validation rather than candidate listing.
	got, err = selector.SelectEntryProfile(context.Background(), "step", []string{"review"}, "", nil, nil)
	if err != nil {
		t.Fatalf("select without executor: %v", err)
	}
	if got != "aaa" {
		t.Fatalf("selected %q, want aaa when no executor context is supplied", got)
	}
}

// unknownUsage returns no telemetry, so the strategy ranks by profile ID.
type unknownUsage struct{}

func (unknownUsage) GetUsage(context.Context, string) (*agentusage.ProviderUsage, error) {
	return nil, nil
}

// exhaustedUsage reports full utilization, so the candidate has zero remaining.
type exhaustedUsage struct{}

func (exhaustedUsage) GetUsage(context.Context, string) (*agentusage.ProviderUsage, error) {
	return &agentusage.ProviderUsage{
		Provider: "test",
		Windows:  []agentusage.UtilizationWindow{{Label: "5-hour", UtilizationPct: 100}},
	}, nil
}

func selectorWithStore(t *testing.T, store stubSelectorStore, provider selection.UsageProvider) *workflowEntryProfileSelector {
	t.Helper()
	reg := agentregistry.NewRegistry(testLogger(t))
	if err := reg.Register(agents.NewClaudeACP()); err != nil {
		t.Fatalf("register agent: %v", err)
	}
	return &workflowEntryProfileSelector{
		settingsStore: store,
		agentRegistry: reg,
		strategy:      &selection.QuotaStrategy{Provider: provider},
		validator:     taskAgentExecutorCompatibilityValidator{profiles: store, agentRegistry: reg},
	}
}

func TestSelectEntryProfileUsesStaticFallbackWhenNoTagMatches(t *testing.T) {
	store := stubSelectorStore{
		agents: []*settingsmodels.Agent{{ID: "a1", Name: "claude-acp"}},
		byAgent: map[string][]*settingsmodels.AgentProfile{
			"a1": {{ID: "profile-other", AgentID: "a1", Enabled: true, Tags: []string{"other"}}},
		},
	}
	selector := selectorWithStore(t, store, unknownUsage{})

	got, err := selector.SelectEntryProfile(context.Background(), "step", []string{"review"}, "profile-fallback", nil, nil)
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if got != "profile-fallback" {
		t.Fatalf("selected %q, want the static fallback", got)
	}
}

func TestSelectEntryProfileErrorsWhenEveryCandidateExhaustedAndNoFallback(t *testing.T) {
	store := stubSelectorStore{
		agents: []*settingsmodels.Agent{{ID: "a1", Name: "claude-acp"}},
		byAgent: map[string][]*settingsmodels.AgentProfile{
			"a1": {{ID: "profile-zero", AgentID: "a1", Enabled: true, Tags: []string{"review"}}},
		},
	}
	selector := selectorWithStore(t, store, exhaustedUsage{})

	_, err := selector.SelectEntryProfile(context.Background(), "step", []string{"review"}, "", nil, nil)
	if !errors.Is(err, taskservice.ErrNoEligibleEntryProfile) {
		t.Fatalf("err = %v, want ErrNoEligibleEntryProfile", err)
	}
}

func TestSelectEntryProfileRejectsExhaustedCandidateAsFallback(t *testing.T) {
	store := stubSelectorStore{
		agents: []*settingsmodels.Agent{{ID: "a1", Name: "claude-acp"}},
		byAgent: map[string][]*settingsmodels.AgentProfile{
			"a1": {{ID: "profile-zero", AgentID: "a1", Enabled: true, Tags: []string{"review"}}},
		},
	}
	selector := selectorWithStore(t, store, exhaustedUsage{})

	_, err := selector.SelectEntryProfile(context.Background(), "step", []string{"review"}, "profile-zero", nil, nil)
	if !errors.Is(err, taskservice.ErrNoEligibleEntryProfile) {
		t.Fatalf("err = %v, want ErrNoEligibleEntryProfile for an exhausted fallback", err)
	}
}
