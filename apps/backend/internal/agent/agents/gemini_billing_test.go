package agents

import (
	"os"
	"path/filepath"
	"testing"

	agentusage "github.com/kandev/kandev/internal/agent/usage"
)

// TestGeminiBillingTypeFromStoredToken proves a stored Gemini CLI token makes
// the agent a subscription account, and an absent/empty file keeps it api_key.
func TestGeminiBillingTypeFromStoredToken(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if got := NewGemini().BillingType(); got != agentusage.BillingTypeAPIKey {
		t.Fatalf("missing token: BillingType() = %q, want api_key", got)
	}

	geminiDir := filepath.Join(home, ".gemini")
	if err := os.MkdirAll(geminiDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	creds := filepath.Join(geminiDir, "oauth_creds.json")
	if err := os.WriteFile(creds, []byte(`{"access_token":"tok","refresh_token":"ref"}`), 0o600); err != nil {
		t.Fatalf("write creds: %v", err)
	}
	if got := NewGemini().BillingType(); got != agentusage.BillingTypeSubscription {
		t.Fatalf("stored token: BillingType() = %q, want subscription", got)
	}
}
