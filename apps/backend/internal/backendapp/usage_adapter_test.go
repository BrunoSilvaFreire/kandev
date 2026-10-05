package backendapp

import (
	"context"
	"errors"
	"testing"
	"time"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	agentusage "github.com/kandev/kandev/internal/agent/usage"
)

type fakeLimitHits struct {
	observed, reset time.Time
	ok              bool
	err             error
}

func (f fakeLimitHits) LatestLimitHit(context.Context, string) (time.Time, time.Time, bool, error) {
	return f.observed, f.reset, f.ok, f.err
}

func TestUsageAdapterActiveLimitHitOverridesFailedFetch(t *testing.T) {
	now := time.Now()
	adapter := &usageProviderAdapter{svc: agentusage.NewUsageService(), limitHits: fakeLimitHits{observed: now, reset: now.Add(time.Hour), ok: true}}
	adapter.svc.Register("p", nil, "openai:key")
	usage, err := adapter.withLimitHit(context.Background(), "p", nil, errors.New("fetch failed"))
	if err != nil || usage == nil || len(usage.Windows) != 1 || usage.Windows[0].UtilizationPct != 100 {
		t.Fatalf("usage/err = %#v/%v", usage, err)
	}
}

func TestUsageAdapterLimitHitExpiresAndNewerFetchWins(t *testing.T) {
	now := time.Now()
	adapter := &usageProviderAdapter{svc: agentusage.NewUsageService(), limitHits: fakeLimitHits{observed: now, reset: now.Add(-time.Minute), ok: true}}
	adapter.svc.Register("p", nil, "openai:key")
	usage := &agentusage.ProviderUsage{FetchedAt: now.Add(time.Minute), Windows: []agentusage.UtilizationWindow{{UtilizationPct: 10}}}
	got, err := adapter.withLimitHit(context.Background(), "p", usage, nil)
	if err != nil || len(got.Windows) != 1 {
		t.Fatalf("usage/err = %#v/%v", got, err)
	}
}

// failingProfileStore fails every profile lookup, standing in for an
// unavailable settings database.
type failingProfileStore struct {
	settingsstore.Repository
}

func (failingProfileStore) GetAgentProfile(context.Context, string) (*settingsmodels.AgentProfile, error) {
	return nil, errors.New("settings unavailable")
}

// TestUsageProviderAdapterProfileLookupFailureIsUnavailable proves a profile
// lookup failure is surfaced as an error (the utilization endpoint's
// unavailable state) rather than collapsed into unknown telemetry.
func TestUsageProviderAdapterProfileLookupFailureIsUnavailable(t *testing.T) {
	adapter := &usageProviderAdapter{settingsStore: failingProfileStore{}}
	usage, err := adapter.GetUsage(context.Background(), "profile-missing")
	if err == nil {
		t.Fatal("profile lookup failure must return an error, not nil,nil")
	}
	if usage != nil {
		t.Fatalf("usage = %#v, want nil", usage)
	}
}

func TestMockUsageFromTags(t *testing.T) {
	profile := &settingsmodels.AgentProfile{Tags: []string{"review", "mock-quota-10"}}
	usage, ok := mockUsageFromTags(profile)
	if !ok || usage == nil || len(usage.Windows) != 1 || usage.Windows[0].UtilizationPct != 10 {
		t.Fatalf("mock usage = %#v, want 10%% utilization", usage)
	}
	if _, ok := mockUsageFromTags(&settingsmodels.AgentProfile{Tags: []string{"review"}}); ok {
		t.Fatal("profile without a mock tag must be unknown")
	}
	if _, ok := mockUsageFromTags(&settingsmodels.AgentProfile{Tags: []string{"mock-quota-nope"}}); ok {
		t.Fatal("malformed mock tag must be ignored")
	}
}

func TestUsageAdapterRegistersAntigravity(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	adapter := &usageProviderAdapter{svc: agentusage.NewUsageService()}
	adapter.ensureRegistered("profile-agy", "agy-acp")

	key, ok := adapter.CacheKeyFor("profile-agy")
	if !ok {
		t.Fatal("expected agy-acp to register a live client")
	}
	if key != agentusage.AntigravityCacheKey() {
		t.Fatalf("cache key = %q, want %q", key, agentusage.AntigravityCacheKey())
	}
}

func TestUsageAdapterEnsureCacheKeyUnresolvable(t *testing.T) {
	adapter := &usageProviderAdapter{svc: agentusage.NewUsageService(), settingsStore: failingProfileStore{}}
	if _, ok := adapter.EnsureCacheKey(context.Background(), "profile-missing"); ok {
		t.Fatal("a failing profile lookup must not resolve a cache key")
	}
}

func TestE2EMockUsageEnabled(t *testing.T) {
	t.Setenv("KANDEV_E2E_MOCK", "true")
	if !e2eMockUsageEnabled() {
		t.Fatal("KANDEV_E2E_MOCK=true must enable mock usage")
	}
	t.Setenv("KANDEV_E2E_MOCK", "")
	if e2eMockUsageEnabled() {
		t.Fatal("empty KANDEV_E2E_MOCK must disable mock usage")
	}
}
