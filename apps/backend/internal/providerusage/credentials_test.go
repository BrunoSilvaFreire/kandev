package providerusage

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	agentusage "github.com/kandev/kandev/internal/agent/usage"
)

func junieProfileAgents() *fakeProfiles {
	return &fakeProfiles{
		agents: []*settingsmodels.Agent{{ID: "junie-acp", Name: "junie-acp"}},
		profiles: map[string][]*settingsmodels.AgentProfile{
			"junie-acp": {{ID: "p1", AgentID: "junie-acp", Name: "Junie"}},
		},
	}
}

func TestOverviewCopiesBalancesAndSubscription(t *testing.T) {
	live := &fakeLive{
		usage: map[string]*agentusage.ProviderUsage{
			"p1": {
				Provider:     "junie",
				Balances:     []agentusage.Balance{{Label: "aip", Amount: 993478.33, Unit: "CREDITS"}},
				Subscription: &agentusage.Subscription{Status: "active", Plan: "AIP"},
			},
		},
		keys: map[string]string{"p1": "junie-account"},
	}
	svc := newOverviewService(t, live, junieProfileAgents())
	resp, err := svc.Overview(context.Background(), "7d")
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	account := resp.Providers[0].Accounts[0]
	if len(account.Balances) != 1 || account.Balances[0].Label != "aip" {
		t.Fatalf("balances = %+v", account.Balances)
	}
	if account.Subscription == nil || account.Subscription.Status != "active" {
		t.Fatalf("subscription = %+v", account.Subscription)
	}
	if account.Status != StatusHealthy {
		t.Fatalf("status = %q, want healthy", account.Status)
	}
}

func TestOverviewZeroBalanceInactiveIsUnknown(t *testing.T) {
	live := &fakeLive{
		usage: map[string]*agentusage.ProviderUsage{
			"p1": {
				Provider:     "junie",
				Balances:     []agentusage.Balance{{Label: "aip", Amount: 0, Unit: "CREDITS"}},
				Subscription: &agentusage.Subscription{Status: "inactive"},
			},
		},
		keys: map[string]string{"p1": "junie-account"},
	}
	svc := newOverviewService(t, live, junieProfileAgents())
	resp, err := svc.Overview(context.Background(), "7d")
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if got := resp.Providers[0].Accounts[0].Status; got == StatusHealthy {
		t.Fatalf("status = %q, want a non-healthy status", got)
	}
}

func TestOverviewCarriesCredentialHint(t *testing.T) {
	hint := &CredentialHint{Kind: CredentialKindJunieAPIKey, AgentType: AgentTypeJunie, SecretName: "junie-api-key", Configured: true}
	live := &fakeLive{
		usage:      map[string]*agentusage.ProviderUsage{"p1": {Provider: "junie", Balances: []agentusage.Balance{{Label: "aip", Amount: 1, Unit: "CREDITS"}}}},
		keys:       map[string]string{"p1": "junie-account"},
		credential: hint,
	}
	svc := newOverviewService(t, live, junieProfileAgents())
	resp, err := svc.Overview(context.Background(), "7d")
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	got := resp.Providers[0].Accounts[0].Credential
	if got == nil || got.SecretName != "junie-api-key" || !got.Configured {
		t.Fatalf("credential = %+v", got)
	}
}

func TestAccountStatusHasLiveAccountState(t *testing.T) {
	status := AccountStatus(StatusInput{HasLiveAccountState: true, Now: time.Now()})
	if status != StatusHealthy {
		t.Fatalf("status = %q, want healthy", status)
	}
}

func TestCredentialsEndpointShape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	live := &fakeLive{hints: []CredentialHint{
		{Kind: CredentialKindOpenCodeConsoleCookie, AgentType: "opencode-acp", SecretName: "opencode-console-cookie", Configured: false},
		{Kind: CredentialKindJunieAPIKey, AgentType: AgentTypeJunie, SecretName: "junie-api-key", Configured: true},
	}}
	svc := newOverviewService(t, live, junieProfileAgents())
	router := gin.New()
	RegisterRoutes(router, svc, testLogger(t))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/provider-usage/credentials", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	var hints []CredentialHint
	if err := json.Unmarshal(recorder.Body.Bytes(), &hints); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(hints) != 2 || hints[0].Kind != CredentialKindOpenCodeConsoleCookie || !hints[1].Configured {
		t.Fatalf("hints = %+v", hints)
	}
	if recorder.Body.String() == "" || json.Valid(recorder.Body.Bytes()) == false {
		t.Fatalf("body = %q", recorder.Body.String())
	}
}
