package backendapp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	agentregistry "github.com/kandev/kandev/internal/agent/registry"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	agentusage "github.com/kandev/kandev/internal/agent/usage"
)

// agentTypeStore models the real id/name split: profile.AgentID is the
// `agents.id` UUID, while the agent record's Name is the type name ("codex-acp",
// "agy-acp", ...). Every usage-path lookup must resolve through this record.
type agentTypeStore struct {
	settingsstore.Repository
	profiles map[string]*settingsmodels.AgentProfile
	agents   map[string]*settingsmodels.Agent
	agentErr error
}

func (s agentTypeStore) GetAgentProfile(_ context.Context, id string) (*settingsmodels.AgentProfile, error) {
	if profile, ok := s.profiles[id]; ok {
		return profile, nil
	}
	return nil, errors.New("profile not found")
}

func (s agentTypeStore) GetAgent(_ context.Context, id string) (*settingsmodels.Agent, error) {
	if s.agentErr != nil {
		return nil, s.agentErr
	}
	if agent, ok := s.agents[id]; ok {
		return agent, nil
	}
	return nil, errors.New("agent not found")
}

func newAgentRegistry(t *testing.T, list ...agents.Agent) *agentregistry.Registry {
	t.Helper()
	reg := agentregistry.NewRegistry(newTestLogger())
	for _, agent := range list {
		if err := reg.Register(agent); err != nil {
			t.Fatalf("register %s: %v", agent.ID(), err)
		}
	}
	return reg
}

// TestUsageAdapterAgentTypeResolvesAgentRecordName locks the fix: the type name
// comes from the settings agent record, not the profile's UUID AgentID.
func TestUsageAdapterAgentTypeResolvesAgentRecordName(t *testing.T) {
	store := agentTypeStore{
		profiles: map[string]*settingsmodels.AgentProfile{"p1": {ID: "p1", AgentID: "53ebb87c-uuid"}},
		agents:   map[string]*settingsmodels.Agent{"53ebb87c-uuid": {ID: "53ebb87c-uuid", Name: "codex-acp"}},
	}
	adapter := &usageProviderAdapter{settingsStore: store}
	got, err := adapter.agentType(context.Background(), store.profiles["p1"])
	if err != nil {
		t.Fatalf("agentType: %v", err)
	}
	if got != "codex-acp" {
		t.Fatalf("agentType = %q, want codex-acp", got)
	}
}

// TestUsageAdapterGetUsageAgentLookupFailureIsUnavailable: a failed agent
// record read is unavailable, never silent unknown telemetry.
func TestUsageAdapterGetUsageAgentLookupFailureIsUnavailable(t *testing.T) {
	store := agentTypeStore{
		profiles: map[string]*settingsmodels.AgentProfile{"p1": {ID: "p1", AgentID: "53ebb87c-uuid"}},
		agentErr: errors.New("agent store unavailable"),
	}
	adapter := &usageProviderAdapter{svc: agentusage.NewUsageService(), settingsStore: store}
	usage, err := adapter.GetUsage(context.Background(), "p1")
	if err == nil {
		t.Fatal("a failed agent lookup must return an error, not nil,nil")
	}
	if usage != nil {
		t.Fatalf("usage = %#v, want nil", usage)
	}
}

// TestGetUsageRegistersResolvedAgentType proves the previously dead registry
// lookup and ensureRegistered switch now fire for a UUID-keyed Codex profile.
func TestGetUsageRegistersResolvedAgentType(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	authDir := filepath.Join(home, ".codex")
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	authPath := filepath.Join(authDir, "auth.json")
	auth := `{"tokens":{"access_token":"test-token","refresh_token":"test-refresh"}}`
	if err := os.WriteFile(authPath, []byte(auth), 0o600); err != nil {
		t.Fatalf("write auth: %v", err)
	}
	store := agentTypeStore{
		profiles: map[string]*settingsmodels.AgentProfile{"p1": {ID: "p1", AgentID: "53ebb87c-uuid"}},
		agents:   map[string]*settingsmodels.Agent{"53ebb87c-uuid": {ID: "53ebb87c-uuid", Name: "codex-acp"}},
	}
	adapter := &usageProviderAdapter{
		svc:           agentusage.NewUsageService(),
		settingsStore: store,
		agentRegistry: newAgentRegistry(t, agents.NewCodexACP()),
	}
	// A cancelled context keeps the post-registration fetch from reaching the
	// network; registration happens before the fetch.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _ = adapter.GetUsage(ctx, "p1")

	key, ok := adapter.CacheKeyFor("p1")
	if !ok {
		t.Fatal("expected the Codex client to be registered for the UUID profile")
	}
	if want := agentusage.CacheKey("openai", authPath); key != want {
		t.Fatalf("cache key = %q, want %q", key, want)
	}
}

// TestQuotaUnavailableReasonUsesResolvedAgentType: the subscription branch must
// be reached for a UUID-keyed profile, so the reason is empty, not no-endpoint.
func TestQuotaUnavailableReasonUsesResolvedAgentType(t *testing.T) {
	store := agentTypeStore{
		profiles: map[string]*settingsmodels.AgentProfile{"p1": {ID: "p1", AgentID: "76c1a25f-uuid"}},
		agents:   map[string]*settingsmodels.Agent{"76c1a25f-uuid": {ID: "76c1a25f-uuid", Name: "agy-acp"}},
	}
	adapter := &usageProviderAdapter{
		svc:           agentusage.NewUsageService(),
		settingsStore: store,
		agentRegistry: newAgentRegistry(t, agents.NewAgyACP()),
		proxyResolver: defaultUsageProxyResolver(),
	}
	if got := adapter.QuotaUnavailableReason(context.Background(), "p1"); got != "" {
		t.Fatalf("reason = %q, want empty for a subscription agent", got)
	}
}

// TestEnsureCacheKeyRegistersResolvedAgentType: the batch/utilization path
// shares the same bug, so it must also resolve the type before registering.
func TestEnsureCacheKeyRegistersResolvedAgentType(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	store := agentTypeStore{
		profiles: map[string]*settingsmodels.AgentProfile{"p1": {ID: "p1", AgentID: "76c1a25f-uuid"}},
		agents:   map[string]*settingsmodels.Agent{"76c1a25f-uuid": {ID: "76c1a25f-uuid", Name: "agy-acp"}},
	}
	adapter := &usageProviderAdapter{
		svc:           agentusage.NewUsageService(),
		settingsStore: store,
		agentRegistry: newAgentRegistry(t, agents.NewAgyACP()),
	}
	key, ok := adapter.EnsureCacheKey(context.Background(), "p1")
	if !ok {
		t.Fatal("expected EnsureCacheKey to resolve the Antigravity client for a UUID profile")
	}
	if key != agentusage.AntigravityCacheKey() {
		t.Fatalf("cache key = %q, want %q", key, agentusage.AntigravityCacheKey())
	}
}

// TestEnsureCacheKeyAgentLookupFailureIsUnknown: an unresolvable agent record
// yields no cache key rather than an error.
func TestEnsureCacheKeyAgentLookupFailureIsUnknown(t *testing.T) {
	store := agentTypeStore{
		profiles: map[string]*settingsmodels.AgentProfile{"p1": {ID: "p1", AgentID: "53ebb87c-uuid"}},
		agentErr: errors.New("agent store unavailable"),
	}
	adapter := &usageProviderAdapter{svc: agentusage.NewUsageService(), settingsStore: store}
	if _, ok := adapter.EnsureCacheKey(context.Background(), "p1"); ok {
		t.Fatal("a failed agent lookup must not resolve a cache key")
	}
}
