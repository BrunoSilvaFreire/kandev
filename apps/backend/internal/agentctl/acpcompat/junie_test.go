package acpcompat

import (
	"testing"
	"time"
)

func TestLateModelOptionWait(t *testing.T) {
	if got := LateModelOptionWait(JunieAgentID); got != 5*time.Second {
		t.Fatalf("LateModelOptionWait(JunieAgentID) = %v, want 5s", got)
	}
	for _, agentID := range []string{CursorAgentID, ""} {
		if got := LateModelOptionWait(agentID); got != 0 {
			t.Errorf("LateModelOptionWait(%q) = %v, want 0", agentID, got)
		}
	}
}
