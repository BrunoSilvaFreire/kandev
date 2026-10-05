package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/selection"
	agentsettingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/agent/settings/store"
	agentusage "github.com/kandev/kandev/internal/agent/usage"
)

type rankingProfiles struct {
	store.Repository
	store.DynamicProfileRepository
	logical  *agentsettingsmodels.AgentProfile
	dynamic  *agentsettingsmodels.DynamicAgentProfile
	concrete map[string]*agentsettingsmodels.AgentProfile
	order    []string
}

func (p *rankingProfiles) GetAgentProfile(_ context.Context, id string) (*agentsettingsmodels.AgentProfile, error) {
	if id == p.logical.ID {
		return p.logical, nil
	}
	if concrete, ok := p.concrete[id]; ok {
		return concrete, nil
	}
	return nil, errors.New("profile not found")
}

func (p *rankingProfiles) GetAgent(_ context.Context, id string) (*agentsettingsmodels.Agent, error) {
	if id == agents.DynamicAgentID {
		return &agentsettingsmodels.Agent{ID: id, Name: agents.DynamicAgentID}, nil
	}
	return &agentsettingsmodels.Agent{ID: id, Name: "concrete"}, nil
}

func (p *rankingProfiles) GetDynamicAgentProfile(
	_ context.Context, profileID string,
) (*agentsettingsmodels.DynamicAgentProfile, []agentsettingsmodels.DynamicAgentRoute, error) {
	if profileID != p.dynamic.ProfileID {
		return nil, nil, errors.New("dynamic profile not found")
	}
	routes := make([]agentsettingsmodels.DynamicAgentRoute, 0, len(p.order))
	for position, id := range p.order {
		routes = append(routes, agentsettingsmodels.DynamicAgentRoute{
			DynamicProfileID: profileID, Position: position, ExecutionProfileID: id, Enabled: true,
		})
	}
	return p.dynamic, routes, nil
}

type fakeUsageProvider struct {
	byID map[string]*agentusage.ProviderUsage
	errs map[string]bool
}

func (f *fakeUsageProvider) GetUsage(_ context.Context, profileID string) (*agentusage.ProviderUsage, error) {
	if f.errs[profileID] {
		return nil, errors.New("usage fetch failed")
	}
	return f.byID[profileID], nil
}

func usageWithRemaining(remainingPct float64) *agentusage.ProviderUsage {
	return &agentusage.ProviderUsage{
		Provider: "test",
		Windows:  []agentusage.UtilizationWindow{{Label: "5h", UtilizationPct: 100 - remainingPct}},
	}
}

func buildRankingResolver(
	dynamicProfile *agentsettingsmodels.DynamicAgentProfile,
	concretes []*agentsettingsmodels.AgentProfile,
	usage selection.UsageProvider,
) *ProfileExecutionResolver {
	byID := make(map[string]*agentsettingsmodels.AgentProfile, len(concretes))
	order := make([]string, 0, len(concretes))
	for _, concrete := range concretes {
		byID[concrete.ID] = concrete
		order = append(order, concrete.ID)
	}
	profiles := &rankingProfiles{
		logical:  &agentsettingsmodels.AgentProfile{ID: "dynamic-profile", AgentID: agents.DynamicAgentID, Enabled: true},
		dynamic:  dynamicProfile,
		concrete: byID,
		order:    order,
	}
	resolver := NewProfileExecutionResolver(profiles, dynamic.NewEngine(), true)
	resolver.SetUsageProvider(usage)
	return resolver
}

func concreteProfile(id string, tags ...string) *agentsettingsmodels.AgentProfile {
	return &agentsettingsmodels.AgentProfile{ID: id, AgentID: "concrete", Name: id, Enabled: true, Tags: tags}
}

func loadRankedProfile(t *testing.T, resolver *ProfileExecutionResolver) dynamic.Profile {
	t.Helper()
	profile, err := resolver.LoadDynamicProfile(context.Background(), "dynamic-profile")
	if err != nil {
		t.Fatalf("LoadDynamicProfile: %v", err)
	}
	return profile
}

func idsOf(profile dynamic.Profile) []string {
	out := make([]string, 0, len(profile.Candidates))
	for _, candidate := range profile.Candidates {
		out = append(out, candidate.ID)
	}
	return out
}

func findCandidate(t *testing.T, profile dynamic.Profile, id string) dynamic.Candidate {
	t.Helper()
	for _, candidate := range profile.Candidates {
		if candidate.ID == id {
			return candidate
		}
	}
	t.Fatalf("candidate %s missing", id)
	return dynamic.Candidate{}
}

func TestDynamicRankingPreferredMatchesEqualCapacity(t *testing.T) {
	usage := &fakeUsageProvider{byID: map[string]*agentusage.ProviderUsage{
		"neutral":   usageWithRemaining(50),
		"preferred": usageWithRemaining(50),
	}}
	resolver := buildRankingResolver(
		&agentsettingsmodels.DynamicAgentProfile{ProfileID: "dynamic-profile", Version: 1, PreferredTags: []string{"claude"}},
		[]*agentsettingsmodels.AgentProfile{concreteProfile("neutral", "codex"), concreteProfile("preferred", "claude")},
		usage,
	)
	profile := loadRankedProfile(t, resolver)
	if got := idsOf(profile); got[0] != "preferred" {
		t.Fatalf("order = %v, want preferred first", got)
	}
	if reason := findCandidate(t, profile, "preferred").Reason; reason != selection.ReasonPreferredTagMatch {
		t.Fatalf("reason = %q", reason)
	}
}

func TestDynamicRankingSkipsZeroPreferredForFallback(t *testing.T) {
	usage := &fakeUsageProvider{byID: map[string]*agentusage.ProviderUsage{
		"preferred": usageWithRemaining(0),
		"neutral":   usageWithRemaining(30),
	}}
	resolver := buildRankingResolver(
		&agentsettingsmodels.DynamicAgentProfile{ProfileID: "dynamic-profile", Version: 1, PreferredTags: []string{"claude"}},
		[]*agentsettingsmodels.AgentProfile{concreteProfile("preferred", "claude"), concreteProfile("neutral", "codex")},
		usage,
	)
	profile := loadRankedProfile(t, resolver)
	if got := idsOf(profile); got[0] != "neutral" {
		t.Fatalf("order = %v, want neutral first", got)
	}
	if candidate := findCandidate(t, profile, "preferred"); candidate.Enabled {
		t.Fatalf("known-zero preferred candidate remained eligible")
	}
	if reason := findCandidate(t, profile, "neutral").Reason; reason != selection.ReasonPreferredUnavailableFallback {
		t.Fatalf("reason = %q", reason)
	}
}

func TestDynamicRankingAvoidedLosesToNeutralButWinsAlone(t *testing.T) {
	usage := &fakeUsageProvider{byID: map[string]*agentusage.ProviderUsage{
		"avoided": usageWithRemaining(60),
		"neutral": usageWithRemaining(60),
	}}
	resolver := buildRankingResolver(
		&agentsettingsmodels.DynamicAgentProfile{ProfileID: "dynamic-profile", Version: 1, AvoidedTags: []string{"gemini"}},
		[]*agentsettingsmodels.AgentProfile{concreteProfile("avoided", "gemini"), concreteProfile("neutral", "codex")},
		usage,
	)
	profile := loadRankedProfile(t, resolver)
	if got := idsOf(profile); got[0] != "neutral" {
		t.Fatalf("order = %v, want neutral first", got)
	}
	if reason := findCandidate(t, profile, "avoided").Reason; reason != selection.ReasonAvoidedOnlyFallback {
		t.Fatalf("avoided reason = %q", reason)
	}

	aloneUsage := &fakeUsageProvider{byID: map[string]*agentusage.ProviderUsage{
		"avoided": usageWithRemaining(60),
	}}
	alone := buildRankingResolver(
		&agentsettingsmodels.DynamicAgentProfile{ProfileID: "dynamic-profile", Version: 1, AvoidedTags: []string{"gemini"}},
		[]*agentsettingsmodels.AgentProfile{concreteProfile("avoided", "gemini")},
		aloneUsage,
	)
	aloneProfile := loadRankedProfile(t, alone)
	if got := idsOf(aloneProfile); got[0] != "avoided" {
		t.Fatalf("order = %v, want avoided first", got)
	}
	if !findCandidate(t, aloneProfile, "avoided").Enabled {
		t.Fatalf("avoided candidate should remain eligible when alone")
	}
}

func TestDynamicRankingEmptyPreferencesUsesCapacityThenOrder(t *testing.T) {
	usage := &fakeUsageProvider{
		byID: map[string]*agentusage.ProviderUsage{
			"first-low":   usageWithRemaining(10),
			"second-high": usageWithRemaining(80),
		},
		errs: map[string]bool{"unknown": true},
	}
	resolver := buildRankingResolver(
		&agentsettingsmodels.DynamicAgentProfile{ProfileID: "dynamic-profile", Version: 1},
		[]*agentsettingsmodels.AgentProfile{
			concreteProfile("first-low", "a"),
			concreteProfile("second-high", "b"),
			concreteProfile("unknown", "c"),
		},
		usage,
	)
	profile := loadRankedProfile(t, resolver)
	got := idsOf(profile)
	if len(got) != 3 || got[2] != "unknown" {
		t.Fatalf("expected unknown last, got %v", got)
	}
	if (got[0] != "second-high" && got[0] != "first-low") || (got[1] != "second-high" && got[1] != "first-low") {
		t.Fatalf("expected positive capacity candidates first, got %v", got)
	}
	if reason := findCandidate(t, profile, "second-high").Reason; reason != selection.ReasonQuotaHeadroom {
		t.Fatalf("positive reason = %q", reason)
	}
	if reason := findCandidate(t, profile, "unknown").Reason; reason != selection.ReasonConfiguredOrder {
		t.Fatalf("unknown reason = %q", reason)
	}
}

func TestDynamicRankingOnlyNonPreferredAvailableSelects(t *testing.T) {
	usage := &fakeUsageProvider{byID: map[string]*agentusage.ProviderUsage{
		"neutral": usageWithRemaining(5),
	}}
	resolver := buildRankingResolver(
		&agentsettingsmodels.DynamicAgentProfile{ProfileID: "dynamic-profile", Version: 1, PreferredTags: []string{"claude"}},
		[]*agentsettingsmodels.AgentProfile{concreteProfile("neutral", "codex")},
		usage,
	)
	profile := loadRankedProfile(t, resolver)
	if got := idsOf(profile); got[0] != "neutral" {
		t.Fatalf("order = %v", got)
	}
	if reason := findCandidate(t, profile, "neutral").Reason; reason != selection.ReasonQuotaHeadroom {
		t.Fatalf("reason = %q", reason)
	}
}

func TestProfileQuotaExhausted(t *testing.T) {
	ctx := context.Background()

	// 1. Nil resolver
	var nilResolver *ProfileExecutionResolver
	if nilResolver.ProfileQuotaExhausted(ctx, "any") {
		t.Fatalf("nil resolver returned true")
	}

	// 2. Nil usage
	resolverNoUsage := buildRankingResolver(
		&agentsettingsmodels.DynamicAgentProfile{ProfileID: "dyn", Version: 1},
		[]*agentsettingsmodels.AgentProfile{concreteProfile("p1", "a")},
		nil,
	)
	if resolverNoUsage.ProfileQuotaExhausted(ctx, "p1") {
		t.Fatalf("resolver with nil usage returned true")
	}

	// 3. Empty profile ID
	if resolverNoUsage.ProfileQuotaExhausted(ctx, "") {
		t.Fatalf("empty profile ID returned true")
	}

	// 4. Usage provider with positive quota, zero quota, and fetch error
	usage := &fakeUsageProvider{
		byID: map[string]*agentusage.ProviderUsage{
			"p-positive": usageWithRemaining(25),
			"p-zero":     usageWithRemaining(0),
			"p-negative": {Provider: "test", Windows: []agentusage.UtilizationWindow{{Label: "5h", UtilizationPct: 110}}},
		},
		errs: map[string]bool{"p-err": true},
	}
	resolver := buildRankingResolver(
		&agentsettingsmodels.DynamicAgentProfile{ProfileID: "dyn", Version: 1},
		[]*agentsettingsmodels.AgentProfile{
			concreteProfile("p-positive", "a"),
			concreteProfile("p-zero", "b"),
			concreteProfile("p-negative", "c"),
			concreteProfile("p-err", "d"),
		},
		usage,
	)

	if resolver.ProfileQuotaExhausted(ctx, "p-positive") {
		t.Fatalf("positive quota reported exhausted")
	}
	if !resolver.ProfileQuotaExhausted(ctx, "p-zero") {
		t.Fatalf("zero quota reported not exhausted")
	}
	if !resolver.ProfileQuotaExhausted(ctx, "p-negative") {
		t.Fatalf("negative quota reported not exhausted")
	}
	if resolver.ProfileQuotaExhausted(ctx, "p-err") {
		t.Fatalf("error quota reported exhausted")
	}
	if resolver.ProfileQuotaExhausted(ctx, "unknown-profile") {
		t.Fatalf("unknown profile reported exhausted")
	}
}
