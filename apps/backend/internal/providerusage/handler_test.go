package providerusage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	agentusage "github.com/kandev/kandev/internal/agent/usage"
	"github.com/kandev/kandev/internal/common/logger"
)

type fakeLive struct {
	usage      map[string]*agentusage.ProviderUsage
	errs       map[string]error
	keys       map[string]string
	hints      []CredentialHint
	credential *CredentialHint
}

func (f *fakeLive) GetUsage(_ context.Context, profileID string) (*agentusage.ProviderUsage, error) {
	if err := f.errs[profileID]; err != nil {
		return nil, err
	}
	return f.usage[profileID], nil
}

func (f *fakeLive) CacheKeyFor(profileID string) (string, bool) {
	key, ok := f.keys[profileID]
	return key, ok
}

func (f *fakeLive) EnsureCacheKey(_ context.Context, profileID string) (string, bool) {
	return f.CacheKeyFor(profileID)
}

func (f *fakeLive) QuotaUnavailableReason(_ context.Context, _ string) string {
	return ""
}

func (f *fakeLive) QuotaCredential(_ context.Context, _ string) *CredentialHint {
	return f.credential
}

func (f *fakeLive) CredentialHints(_ context.Context) []CredentialHint {
	return f.hints
}

func (f *fakeLive) InvalidateQuotaCache() {}

type fakeProfiles struct {
	agents   []*settingsmodels.Agent
	profiles map[string][]*settingsmodels.AgentProfile
}

func (f *fakeProfiles) ListAgents(context.Context) ([]*settingsmodels.Agent, error) {
	return f.agents, nil
}

func (f *fakeProfiles) ListAgentProfiles(_ context.Context, agentID string) ([]*settingsmodels.AgentProfile, error) {
	return f.profiles[agentID], nil
}

func testLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	return log
}

func newOverviewService(t *testing.T, live LiveProvider, profiles ProfileSource) *Service {
	t.Helper()
	repo := newTestRepository(t)
	indexer := NewIndexer(repo, &fakeLedger{}, staticResolver{}, Accounts{}, nil)
	return NewService(repo, live, profiles, Accounts{Anthropic: "anthropic-key", OpenAI: "openai-key"}, indexer, testLogger(t))
}

func twoProfileAgents() *fakeProfiles {
	return &fakeProfiles{
		agents: []*settingsmodels.Agent{{ID: "claude-acp", Name: "claude-acp"}},
		profiles: map[string][]*settingsmodels.AgentProfile{
			"claude-acp": {
				{ID: "p1", AgentID: "claude-acp", Name: "One"},
				{ID: "p2", AgentID: "claude-acp", Name: "Two"},
			},
		},
	}
}

// TestOverviewGroupsByResolvedAgentTypeNotAgentUUID locks the overview half of
// the usage fix: a profile whose AgentID is the agents.id UUID must group under
// the provider derived from the agent record's Name, not the UUID.
func TestOverviewGroupsByResolvedAgentTypeNotAgentUUID(t *testing.T) {
	live := &fakeLive{
		usage: map[string]*agentusage.ProviderUsage{
			"p1": {Provider: "openai", Windows: []agentusage.UtilizationWindow{{Label: "5-hour", UtilizationPct: 100}}},
		},
		keys: map[string]string{"p1": "openai-key"},
	}
	profiles := &fakeProfiles{
		agents: []*settingsmodels.Agent{{ID: "53ebb87c-uuid", Name: "codex-acp"}},
		profiles: map[string][]*settingsmodels.AgentProfile{
			"53ebb87c-uuid": {{ID: "p1", AgentID: "53ebb87c-uuid", Name: "GPT-5.6 Sol"}},
		},
	}
	svc := newOverviewService(t, live, profiles)

	resp, err := svc.Overview(context.Background(), "7d")
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if len(resp.Providers) != 1 || resp.Providers[0].Provider != ProviderOpenAI {
		t.Fatalf("expected one openai provider, got %+v", resp.Providers)
	}
	if resp.Providers[0].DisplayName != "OpenAI" {
		t.Fatalf("display name = %q, want OpenAI", resp.Providers[0].DisplayName)
	}
}

func TestOverviewGroupsProfilesByAccount(t *testing.T) {
	now := time.Now().UTC()
	live := &fakeLive{
		usage: map[string]*agentusage.ProviderUsage{
			"p1": {Provider: "anthropic", Plan: "max", Windows: []agentusage.UtilizationWindow{{Label: "5-hour", UtilizationPct: 42, ResetAt: now.Add(time.Hour)}}},
			"p2": {Provider: "anthropic", Plan: "max", Windows: []agentusage.UtilizationWindow{{Label: "5-hour", UtilizationPct: 42, ResetAt: now.Add(time.Hour)}}},
		},
		keys: map[string]string{"p1": "shared", "p2": "shared"},
	}
	svc := newOverviewService(t, live, twoProfileAgents())

	resp, err := svc.Overview(context.Background(), "7d")
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if len(resp.Providers) != 1 || len(resp.Providers[0].Accounts) != 1 {
		t.Fatalf("expected one provider with one account, got %+v", resp.Providers)
	}
	account := resp.Providers[0].Accounts[0]
	if account.AccountKey != "shared" || len(account.Profiles) != 2 {
		t.Fatalf("expected deduped account with 2 profiles, got %+v", account)
	}
	if account.Status != StatusHealthy {
		t.Fatalf("expected healthy, got %q", account.Status)
	}
	if len(account.Windows) != 1 || account.Windows[0].Estimate == nil {
		t.Fatalf("expected one window with an estimate, got %+v", account.Windows)
	}
}

func TestOverviewErrorIsUnavailable(t *testing.T) {
	live := &fakeLive{
		errs: map[string]error{"p1": errors.New("antigravity not running")},
		keys: map[string]string{"p1": "agy"},
	}
	profiles := &fakeProfiles{
		agents:   []*settingsmodels.Agent{{ID: "agy-acp", Name: "agy-acp"}},
		profiles: map[string][]*settingsmodels.AgentProfile{"agy-acp": {{ID: "p1", AgentID: "agy-acp", Name: "Agy"}}},
	}
	svc := newOverviewService(t, live, profiles)

	resp, err := svc.Overview(context.Background(), "24h")
	if err != nil {
		t.Fatalf("overview must not fail on a provider error: %v", err)
	}
	if resp.Providers[0].Accounts[0].Status != StatusUnavailable {
		t.Fatalf("expected unavailable, got %q", resp.Providers[0].Accounts[0].Status)
	}
	if resp.Range != "24h" {
		t.Fatalf("range = %q, want 24h", resp.Range)
	}
}

func TestOverviewStatusThresholds(t *testing.T) {
	now := time.Now().UTC()
	live := &fakeLive{
		usage: map[string]*agentusage.ProviderUsage{
			"p1": {Provider: "anthropic", Windows: []agentusage.UtilizationWindow{{Label: "5-hour", UtilizationPct: 85, ResetAt: now.Add(time.Hour)}}},
		},
		keys: map[string]string{"p1": "acct"},
	}
	profiles := &fakeProfiles{
		agents:   []*settingsmodels.Agent{{ID: "claude-acp", Name: "claude-acp"}},
		profiles: map[string][]*settingsmodels.AgentProfile{"claude-acp": {{ID: "p1", AgentID: "claude-acp", Name: "One"}}},
	}
	svc := newOverviewService(t, live, profiles)

	resp, _ := svc.Overview(context.Background(), "")
	if got := resp.Providers[0].Accounts[0].Status; got != StatusNearing {
		t.Fatalf("expected nearing, got %q", got)
	}
	if resp.Range != Range7d {
		t.Fatalf("default range = %q, want 7d", resp.Range)
	}
}

func TestOverviewUnknownAgentIncluded(t *testing.T) {
	live := &fakeLive{}
	profiles := &fakeProfiles{
		agents:   []*settingsmodels.Agent{{ID: "gemini", Name: "gemini"}},
		profiles: map[string][]*settingsmodels.AgentProfile{"gemini": {{ID: "g1", AgentID: "gemini", Name: "Gem"}}},
	}
	svc := newOverviewService(t, live, profiles)

	resp, _ := svc.Overview(context.Background(), "7d")
	if len(resp.Providers) != 1 || resp.Providers[0].Provider != ProviderGoogle {
		t.Fatalf("expected a Google provider, got %+v", resp.Providers)
	}
	if resp.Providers[0].Accounts[0].Status != StatusUnknown {
		t.Fatalf("expected unknown, got %q", resp.Providers[0].Accounts[0].Status)
	}
}

func newTestEngine(t *testing.T, svc *Service) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterRoutes(router, svc, testLogger(t))
	return router
}

func TestUpdateSourcesRejectsInvalid(t *testing.T) {
	svc := newOverviewService(t, &fakeLive{}, twoProfileAgents())
	router := newTestEngine(t, svc)

	for _, body := range []string{`{"kandev":true}`, `{"bogus":true}`} {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/provider-usage/sources", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("body %s: status = %d, want 400", body, recorder.Code)
		}
	}
}

func TestUpdateSourcesEnablesAndDisables(t *testing.T) {
	svc := newOverviewService(t, &fakeLive{}, twoProfileAgents())
	router := newTestEngine(t, svc)

	put := func(body string) sourcesResponse {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/provider-usage/sources", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, body %s", recorder.Code, recorder.Body.String())
		}
		var parsed sourcesResponse
		if err := json.Unmarshal(recorder.Body.Bytes(), &parsed); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return parsed
	}

	enabled := put(`{"codex_local":true}`)
	if !sourceEnabled(enabled.Sources, SourceCodexLocal) {
		t.Fatalf("expected codex_local enabled: %+v", enabled.Sources)
	}
	disabled := put(`{"codex_local":false}`)
	if sourceEnabled(disabled.Sources, SourceCodexLocal) {
		t.Fatalf("expected codex_local disabled: %+v", disabled.Sources)
	}
	if !sourceEnabled(disabled.Sources, SourceKandev) {
		t.Fatal("kandev must always be enabled")
	}
}

type sourcesResponse struct {
	Sources []SourceView `json:"sources"`
}

func sourceEnabled(sources []SourceView, name string) bool {
	for _, source := range sources {
		if source.Source == name {
			return source.Enabled
		}
	}
	return false
}
