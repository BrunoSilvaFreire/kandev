package backendapp

import (
	"context"
	"errors"
	"testing"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
)

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
