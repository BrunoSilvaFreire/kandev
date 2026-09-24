package backendapp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	agentregistry "github.com/kandev/kandev/internal/agent/registry"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	agentusage "github.com/kandev/kandev/internal/agent/usage"
)

// mockUsageTagPrefix marks an E2E-only profile tag carrying a deterministic
// provider utilization percentage (for example "mock-quota-10" means 10%
// utilization, 90% remaining). It is read only under the e2e mock profile so
// production telemetry can never be faked.
const mockUsageTagPrefix = "mock-quota-"

func e2eMockUsageEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("KANDEV_E2E_MOCK"))) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

func mockUsageFromTags(profile *settingsmodels.AgentProfile) (*agentusage.ProviderUsage, bool) {
	if profile == nil {
		return nil, false
	}
	for _, tag := range profile.Tags {
		if !strings.HasPrefix(tag, mockUsageTagPrefix) {
			continue
		}
		pct, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimPrefix(tag, mockUsageTagPrefix)), 64)
		if err != nil {
			continue
		}
		if pct < 0 {
			pct = 0
		}
		if pct > 100 {
			pct = 100
		}
		return &agentusage.ProviderUsage{
			Provider: "mock",
			Windows: []agentusage.UtilizationWindow{{
				Label:          "5-hour",
				UtilizationPct: pct,
				ResetAt:        time.Now().UTC().Add(time.Hour),
			}},
		}, true
	}
	return nil, false
}

// usageProviderAdapter implements officeagents.UsageProvider by:
//  1. Looking up the agent profile by ID from the settings store.
//  2. Looking up the agent type from the registry to get its billing type.
//  3. Delegating to the UsageService with the appropriate client registered.
type usageProviderAdapter struct {
	svc           *agentusage.UsageService
	settingsStore settingsstore.Repository
	agentRegistry *agentregistry.Registry
	proxyResolver usageProxyResolver
}

// GetUsage implements officeagents.UsageProvider.
func (a *usageProviderAdapter) GetUsage(ctx context.Context, profileID string) (*agentusage.ProviderUsage, error) {
	profile, err := a.settingsStore.GetAgentProfile(ctx, profileID)
	if err != nil {
		// A profile lookup failure is unavailable, not unknown: the consumer
		// can distinguish "no telemetry" from "could not resolve the profile"
		// and keeps the three-state contract.
		return nil, fmt.Errorf("load agent profile %s for usage: %w", profileID, err)
	}
	if e2eMockUsageEnabled() {
		if usage, ok := mockUsageFromTags(profile); ok {
			return usage, nil
		}
		return nil, nil
	}
	ag, ok := a.agentRegistry.Get(profile.AgentID)
	if !ok {
		return nil, nil
	}
	if a.proxyResolver != nil {
		if client, cacheKey, ok := a.proxyResolver.Resolve(profile); ok {
			a.svc.Register(profileID, client, cacheKey)
			return a.svc.GetUsage(ctx, profileID)
		}
	}
	if ag.BillingType() != agentusage.BillingTypeSubscription {
		return nil, nil
	}
	// Ensure client is registered for this profile.
	a.ensureRegistered(profileID, profile.AgentID)
	return a.svc.GetUsage(ctx, profileID)
}

// ensureRegistered creates and registers a usage client for the profile if not already registered.
func (a *usageProviderAdapter) ensureRegistered(profileID, agentName string) {
	// We rely on the fact that GetUsage returns nil,nil for unregistered profiles,
	// so we can call Register without causing duplicate cache entries — the cache key
	// is credential-path-based, not profileID-based, so two profiles with the same
	// credentials share one cache entry.
	//
	// `home` is required for both branches — bail rather than registering a
	// client pointed at a relative ".claude/.credentials.json" / ".codex/auth.json"
	// that silently misses the real file (common in containers or when HOME is
	// unset). Without this guard the consumer sees BillingTypeAPIKey forever.
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	switch agentName {
	case claudeACPAgentID:
		credPath := filepath.Join(home, ".claude", ".credentials.json")
		client := agentusage.NewClaudeUsageClientWithPath(credPath)
		key := agentusage.CacheKey("anthropic", credPath)
		a.svc.Register(profileID, client, key)
	case "codex-acp":
		// Path must match codex_acp.go's SourceFiles / Runtime mounts —
		// the real Codex CLI persists OAuth tokens at ~/.codex/auth.json,
		// not the earlier XDG-style ~/.config/codex/ guess.
		authPath := filepath.Join(home, ".codex", "auth.json")
		client := agentusage.NewCodexUsageClientWithPath(authPath)
		key := agentusage.CacheKey("openai", authPath)
		a.svc.Register(profileID, client, key)
	}
}

// newUsageProviderAdapter creates an adapter and returns it.
func newUsageProviderAdapter(
	settingsStore settingsstore.Repository,
	agentRegistry *agentregistry.Registry,
) *usageProviderAdapter {
	return &usageProviderAdapter{
		svc:           agentusage.NewUsageService(),
		settingsStore: settingsStore,
		agentRegistry: agentRegistry,
		proxyResolver: defaultUsageProxyResolver(),
	}
}
