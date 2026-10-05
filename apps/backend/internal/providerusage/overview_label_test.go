package providerusage

import "testing"

func TestAccountLabelDerivation(t *testing.T) {
	profiles := []ProfileRef{{ID: "p1", Name: "Work"}, {ID: "p2", Name: "Personal"}}
	unnamed := []ProfileRef{{ID: "p1"}}
	claudeLocal := []Observation{{Source: SourceClaudeLocal}}
	codexLocal := []Observation{{Source: SourceCodexLocal}}

	cases := []struct {
		name         string
		plan         string
		profiles     []ProfileRef
		observations []Observation
		want         string
	}{
		{"plan and profile", "max", profiles, nil, "max \u00b7 Work"},
		{"plan only", "max", nil, nil, "max"},
		{"profile only", "", profiles, nil, "Work"},
		{"unnamed profile falls through", "", unnamed, nil, ""},
		{"claude local history", "", nil, claudeLocal, "Claude Code local history"},
		{"codex local history", "", nil, codexLocal, "Codex local history"},
		{"no signal", "", nil, nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := accountLabel(tc.plan, tc.profiles, tc.observations); got != tc.want {
				t.Fatalf("accountLabel() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestProviderDisplayNameResolution(t *testing.T) {
	cases := []struct {
		provider  string
		agentName string
		want      string
	}{
		{ProviderAnthropic, "ignored", "Anthropic"},
		{ProviderOpenAI, "ignored", "OpenAI"},
		{ProviderAntigravity, "ignored", "Antigravity"},
		{ProviderGoogle, "ignored", "Google"},
		{ProviderOpenCodeGo, "ignored", "OpenCode Go"},
		{"my-plugin-agent", "My Plugin", "My Plugin"},
		{"my-plugin-agent", "", "my-plugin-agent"},
	}
	for _, tc := range cases {
		t.Run(tc.provider+"/"+tc.agentName, func(t *testing.T) {
			if got := providerDisplayName(tc.provider, tc.agentName); got != tc.want {
				t.Fatalf("providerDisplayName() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestProviderForProfile(t *testing.T) {
	if got := providerForProfile("opencode-acp", "opencode-go/deepseek"); got != ProviderOpenCodeGo {
		t.Fatalf("provider = %q, want %q", got, ProviderOpenCodeGo)
	}
	if got := providerForProfile(AgentTypeGemini, "gemini-pro"); got != ProviderGoogle {
		t.Fatalf("provider = %q, want %q", got, ProviderGoogle)
	}
}
