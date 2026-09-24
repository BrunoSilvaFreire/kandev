package store

import (
	"context"
	"strings"
	"testing"
)

// TestSeedBuiltinAgents_SeedsExtractResumeHandoff pins the resume-handoff
// built-in: it must be seeded with the embedded prompt so the frontend can
// execute it by ID without any user configuration.
func TestSeedBuiltinAgents_SeedsExtractResumeHandoff(t *testing.T) {
	sqlxDB := openTestDB(t)

	repo, err := newSQLiteRepositoryWithDB(sqlxDB, sqlxDB)
	if err != nil {
		t.Fatalf("schema init: %v", err)
	}

	agent, err := repo.GetAgentByID(context.Background(), "builtin-extract-resume-handoff")
	if err != nil {
		t.Fatalf("GetAgentByID: %v", err)
	}
	if !agent.Builtin {
		t.Errorf("builtin = false, want true")
	}
	if agent.Name != "extract-resume-handoff" {
		t.Errorf("name = %q, want %q", agent.Name, "extract-resume-handoff")
	}
	if strings.TrimSpace(agent.Prompt) == "" {
		t.Fatalf("prompt is empty")
	}
	if !strings.Contains(agent.Prompt, "{{ConversationHistory}}") {
		t.Errorf("prompt does not contain the {{ConversationHistory}} placeholder")
	}
}
