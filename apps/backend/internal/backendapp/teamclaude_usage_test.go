package backendapp

import (
	"testing"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
)

func TestTeamClaudeStatusURL(t *testing.T) {
	cases := []struct {
		name      string
		base      string
		vendor    string
		agentType string
		want      string
		ok        bool
	}{
		{"localhost", "http://localhost:3456", "teamclaude", "claude-acp", "http://localhost:3456/teamclaude/status", true},
		{"ipv6 loopback", "http://[::1]:3456/v1", "teamclaude", "claude-acp", "http://[::1]:3456/teamclaude/status", true},
		{"selection required", "http://localhost:3456", "", "claude-acp", "", false},
		{"remote denied", "https://proxy.example.test", "teamclaude", "claude-acp", "", false},
		{"credentials denied", "http://user:" +
			"PASSWORD@localhost:3456", "teamclaude", "claude-acp", "", false},
		{"not a URL", "localhost:3456", "teamclaude", "claude-acp", "", false},
		{"other agent type", "http://localhost:3456", "teamclaude", "codex-acp", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := teamClaudeStatusURL(&settingsmodels.AgentProfile{
				AgentID: "53ebb87c-uuid",
				EnvVars: []settingsmodels.ProfileEnvVar{
					{Key: teamClaudeBaseURLEnv, Value: tc.base},
					{Key: usageProxyVendorEnv, Value: tc.vendor},
				},
			}, tc.agentType)
			if got != tc.want || ok != tc.ok {
				t.Fatalf("teamClaudeStatusURL = %q, %v; want %q, %v", got, ok, tc.want, tc.ok)
			}
		})
	}
}
