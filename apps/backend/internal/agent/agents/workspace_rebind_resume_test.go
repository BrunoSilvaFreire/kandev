package agents

import "testing"

func TestWorkspaceRebindSessionCompatibility(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		agent            Agent
		startsNewSession bool
	}{
		{name: "codex loads session in changed cwd", agent: NewCodexACP()},
		{name: "cursor loads session in changed cwd", agent: NewCursorACP()},
		{name: "opencode starts fresh session", agent: NewOpenCodeACP(), startsNewSession: true},
		{name: "grok starts fresh session", agent: NewGrokACP(), startsNewSession: true},
		// Junie scopes a session to its project directory: session/load with a
		// changed cwd returns no session model config, so a rebind must start
		// a new session.
		{name: "junie starts fresh session", agent: NewJunieACP(), startsNewSession: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got := test.agent.Runtime().SessionConfig.NewSessionOnWorkspaceRebind
			if got != test.startsNewSession {
				t.Fatalf("NewSessionOnWorkspaceRebind = %v, want %v", got, test.startsNewSession)
			}
		})
	}
}
