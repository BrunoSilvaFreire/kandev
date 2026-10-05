package backendapp

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	agentusage "github.com/kandev/kandev/internal/agent/usage"
)

func goProfileStore(profiles map[string]*settingsmodels.AgentProfile) agentTypeStore {
	return agentTypeStore{
		profiles: profiles,
		agents: map[string]*settingsmodels.Agent{
			"oc-uuid": {ID: "oc-uuid", Name: openCodeACPAgentType},
		},
	}
}

// TestEnsureCacheKeyOpenCodeGoWithoutCookie: the Go client is registered under
// the account-wide Go key even when no cookie function exists, because the
// missing cookie is exactly what the limit-hit fallback covers.
func TestEnsureCacheKeyOpenCodeGoWithoutCookie(t *testing.T) {
	store := goProfileStore(map[string]*settingsmodels.AgentProfile{
		"p-go": {ID: "p-go", AgentID: "oc-uuid", Model: "opencode-go/deepseek"},
	})
	adapter := &usageProviderAdapter{
		svc:           agentusage.NewUsageService(),
		settingsStore: store,
		agentRegistry: newAgentRegistry(t, agents.NewOpenCodeACP()),
	}
	key, ok := adapter.EnsureCacheKey(context.Background(), "p-go")
	if !ok {
		t.Fatal("EnsureCacheKey must resolve the Go account key without a cookie")
	}
	if key != agentusage.OpenCodeGoCacheKey() {
		t.Fatalf("cache key = %q, want %q", key, agentusage.OpenCodeGoCacheKey())
	}
}

// TestEnsureCacheKeyOpenCodeGoSharesOneKey: two Go profiles share the single
// account-wide cache key.
func TestEnsureCacheKeyOpenCodeGoSharesOneKey(t *testing.T) {
	store := goProfileStore(map[string]*settingsmodels.AgentProfile{
		"p1": {ID: "p1", AgentID: "oc-uuid", Model: "opencode-go/deepseek"},
		"p2": {ID: "p2", AgentID: "oc-uuid", Model: "opencode-go/qwen"},
	})
	adapter := &usageProviderAdapter{
		svc:           agentusage.NewUsageService(),
		settingsStore: store,
		agentRegistry: newAgentRegistry(t, agents.NewOpenCodeACP()),
	}
	k1, ok1 := adapter.EnsureCacheKey(context.Background(), "p1")
	k2, ok2 := adapter.EnsureCacheKey(context.Background(), "p2")
	if !ok1 || !ok2 {
		t.Fatalf("both Go profiles must resolve a key: %v/%v", ok1, ok2)
	}
	if k1 != k2 || k1 != agentusage.OpenCodeGoCacheKey() {
		t.Fatalf("Go keys = %q/%q, want one shared %q", k1, k2, agentusage.OpenCodeGoCacheKey())
	}
}

// TestEnsureCacheKeyNonGoOpenCodeHasNoKey: an opencode-acp profile outside the
// opencode-go namespace is API-key billing and gets no live quota key.
func TestEnsureCacheKeyNonGoOpenCodeHasNoKey(t *testing.T) {
	store := goProfileStore(map[string]*settingsmodels.AgentProfile{
		"p-api": {ID: "p-api", AgentID: "oc-uuid", Model: "openai/gpt-5"},
	})
	adapter := &usageProviderAdapter{
		svc:           agentusage.NewUsageService(),
		settingsStore: store,
		agentRegistry: newAgentRegistry(t, agents.NewOpenCodeACP()),
	}
	if key, ok := adapter.EnsureCacheKey(context.Background(), "p-api"); ok {
		t.Fatalf("non-Go OpenCode profile resolved a key %q, want none", key)
	}
}

// TestOpenCodeGoActiveLimitHitWithoutCookie: with no cookie the Go fetch fails
// with ErrCredentialsMissing, so the recorded limit hit is what marks the
// account exhausted as a 100% window.
func TestOpenCodeGoActiveLimitHitWithoutCookie(t *testing.T) {
	now := time.Now()
	store := goProfileStore(map[string]*settingsmodels.AgentProfile{
		"p-go": {ID: "p-go", AgentID: "oc-uuid", Model: "opencode-go/deepseek"},
	})
	adapter := &usageProviderAdapter{
		svc:           agentusage.NewUsageService(),
		settingsStore: store,
		agentRegistry: newAgentRegistry(t, agents.NewOpenCodeACP()),
		limitHits:     fakeLimitHits{observed: now, reset: now.Add(time.Hour), ok: true},
	}
	usage, err := adapter.GetUsage(context.Background(), "p-go")
	if err != nil {
		t.Fatalf("GetUsage: %v", err)
	}
	if usage == nil || len(usage.Windows) != 1 {
		t.Fatalf("usage = %#v, want one synthetic window", usage)
	}
	if usage.Windows[0].Label != "limit" || usage.Windows[0].UtilizationPct != 100 {
		t.Fatalf("window = %#v, want a 100%% limit window", usage.Windows[0])
	}
}
