package backendapp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	agentusage "github.com/kandev/kandev/internal/agent/usage"
	"github.com/kandev/kandev/internal/providerusage"
)

type expiredUsageClient struct{}

func (expiredUsageClient) FetchUsage(context.Context) (*agentusage.ProviderUsage, error) {
	return nil, agentusage.ErrCredentialsExpired
}

func writeJunieCredentialFile(t *testing.T, home string, token string) {
	t.Helper()
	dir := filepath.Join(home, ".junie")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	inner, err := json.Marshal(map[string]any{"jbAccount": map[string]any{"access_token": token, "refresh_token": "r"}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	outer, err := json.Marshal(map[string]any{"secrets": []map[string]string{{"key": "jb-account-stored", "secret": string(inner)}}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "secure_credentials.json"), outer, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func unsignedJWT(t *testing.T, exp time.Time) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"ES256"}`))
	claims, err := json.Marshal(map[string]any{"exp": exp.Unix()})
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	return header + "." + base64.RawURLEncoding.EncodeToString(claims) + ".sig"
}

func TestUsageAdapterJunieSpecialClient(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	adapter := &usageProviderAdapter{svc: agentusage.NewUsageService()}
	client, key, ok := adapter.specialClient(&settingsmodels.AgentProfile{Model: "junie"}, providerusage.AgentTypeJunie)
	if !ok || client == nil {
		t.Fatal("expected a Junie special client")
	}
	if key != agentusage.JunieCacheKey() {
		t.Fatalf("cache key = %q, want %q", key, agentusage.JunieCacheKey())
	}
}

func TestUsageAdapterJunieReasonMissing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	adapter := &usageProviderAdapter{svc: agentusage.NewUsageService()}
	if got := adapter.junieReason(context.Background()); got != providerusage.QuotaUnavailableCredentialsMissing {
		t.Fatalf("reason = %q, want missing", got)
	}
}

func TestUsageAdapterJunieReasonExpired(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeJunieCredentialFile(t, home, unsignedJWT(t, time.Now().Add(-time.Minute)))
	adapter := &usageProviderAdapter{svc: agentusage.NewUsageService()}
	if got := adapter.junieReason(context.Background()); got != providerusage.QuotaUnavailableCredentialsExpired {
		t.Fatalf("reason = %q, want expired", got)
	}
}

func TestUsageAdapterJunieReasonConfigured(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	adapter := &usageProviderAdapter{
		svc:         agentusage.NewUsageService(),
		junieAPIKey: func(context.Context) (string, error) { return "junie-key", nil },
	}
	if got := adapter.junieReason(context.Background()); got != "" {
		t.Fatalf("reason = %q, want empty", got)
	}
}

func TestUsageAdapterJunieReasonUnexpiredFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeJunieCredentialFile(t, home, unsignedJWT(t, time.Now().Add(time.Hour)))
	adapter := &usageProviderAdapter{svc: agentusage.NewUsageService()}
	if got := adapter.junieReason(context.Background()); got != "" {
		t.Fatalf("reason = %q, want empty", got)
	}
}

func TestUsageAdapterOpenCodeGoExpiredReason(t *testing.T) {
	adapter := &usageProviderAdapter{
		svc:            agentusage.NewUsageService(),
		opencodeCookie: func(context.Context) (string, error) { return "session=cookie", nil },
	}
	adapter.svc.Register("p", expiredUsageClient{}, agentusage.OpenCodeGoCacheKey())
	if _, err := adapter.svc.GetUsage(context.Background(), "p"); err == nil {
		t.Fatal("expected a fetch error")
	}
	if got := adapter.openCodeGoReason(context.Background()); got != providerusage.QuotaUnavailableCredentialsExpired {
		t.Fatalf("reason = %q, want expired", got)
	}
}

func TestUsageAdapterOpenCodeGoMissingReason(t *testing.T) {
	adapter := &usageProviderAdapter{svc: agentusage.NewUsageService()}
	if got := adapter.openCodeGoReason(context.Background()); got != providerusage.QuotaUnavailableCredentialsMissing {
		t.Fatalf("reason = %q, want missing", got)
	}
}
