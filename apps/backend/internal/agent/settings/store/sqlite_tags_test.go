package store

import (
	"context"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/agent/settings/models"
)

func newTagsTestRepo(t *testing.T) *sqliteRepository {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo, err := newSQLiteRepository(db, db, nil, false)
	if err != nil {
		t.Fatalf("newSQLiteRepository: %v", err)
	}
	return repo
}

func TestAgentProfileTagsRoundTrip(t *testing.T) {
	repo := newTagsTestRepo(t)
	ctx := context.Background()
	agent := &models.Agent{Name: "claude"}
	if err := repo.CreateAgent(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	profile := &models.AgentProfile{
		AgentID:          agent.ID,
		Name:             "tagged",
		AgentDisplayName: "Tagged Profile",
		Tags:             []string{"review", "security"},
	}
	if err := repo.CreateAgentProfile(ctx, profile); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	got, err := repo.GetAgentProfile(ctx, profile.ID)
	if err != nil {
		t.Fatalf("get profile: %v", err)
	}
	if len(got.Tags) != 2 || got.Tags[0] != "review" || got.Tags[1] != "security" {
		t.Fatalf("tags = %#v, want [review security]", got.Tags)
	}

	got.Tags = []string{"ops"}
	if err := repo.UpdateAgentProfile(ctx, got); err != nil {
		t.Fatalf("update profile: %v", err)
	}
	reloaded, err := repo.GetAgentProfile(ctx, profile.ID)
	if err != nil {
		t.Fatalf("reload profile: %v", err)
	}
	if len(reloaded.Tags) != 1 || reloaded.Tags[0] != "ops" {
		t.Fatalf("tags = %#v, want [ops]", reloaded.Tags)
	}
}

// TestAgentProfileTagsCanonicalizedAtRepositoryBoundary proves a direct
// repository write canonicalizes tags (trim, lowercase, dedupe, sort) even
// when it bypasses the controller.
func TestAgentProfileTagsCanonicalizedAtRepositoryBoundary(t *testing.T) {
	repo := newTagsTestRepo(t)
	ctx := context.Background()
	agent := &models.Agent{Name: "claude"}
	if err := repo.CreateAgent(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	profile := &models.AgentProfile{
		AgentID:          agent.ID,
		Name:             "tagged",
		AgentDisplayName: "Tagged Profile",
		Tags:             []string{" Review ", "security", "REVIEW"},
	}
	if err := repo.CreateAgentProfile(ctx, profile); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	got, err := repo.GetAgentProfile(ctx, profile.ID)
	if err != nil {
		t.Fatalf("get profile: %v", err)
	}
	if len(got.Tags) != 2 || got.Tags[0] != "review" || got.Tags[1] != "security" {
		t.Fatalf("tags = %#v, want canonical [review security]", got.Tags)
	}

	got.Tags = []string{" Ops ", "ops", "build"}
	if err := repo.UpdateAgentProfile(ctx, got); err != nil {
		t.Fatalf("update profile: %v", err)
	}
	reloaded, err := repo.GetAgentProfile(ctx, profile.ID)
	if err != nil {
		t.Fatalf("reload profile: %v", err)
	}
	if len(reloaded.Tags) != 2 || reloaded.Tags[0] != "build" || reloaded.Tags[1] != "ops" {
		t.Fatalf("tags = %#v, want canonical [build ops]", reloaded.Tags)
	}
}

// TestAgentProfileTagsRejectInvalidAtRepositoryBoundary proves direct writes
// reject an oversized tag instead of persisting it.
func TestAgentProfileTagsRejectInvalidAtRepositoryBoundary(t *testing.T) {
	repo := newTagsTestRepo(t)
	ctx := context.Background()
	agent := &models.Agent{Name: "claude"}
	if err := repo.CreateAgent(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	profile := &models.AgentProfile{
		AgentID:          agent.ID,
		Name:             "bad-tag",
		AgentDisplayName: "Bad Tag",
		Tags:             []string{strings.Repeat("x", 65)},
	}
	if err := repo.CreateAgentProfile(ctx, profile); err == nil {
		t.Fatal("oversized tag must be rejected at the repository boundary")
	}
	if _, err := repo.GetAgentProfile(ctx, profile.ID); err == nil {
		t.Fatal("rejected profile must not be persisted")
	}
}

func TestAgentProfileTagsDefaultToEmpty(t *testing.T) {
	repo := newTagsTestRepo(t)
	ctx := context.Background()
	agent := &models.Agent{Name: "claude"}
	if err := repo.CreateAgent(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	profile := &models.AgentProfile{
		AgentID:          agent.ID,
		Name:             "untagged",
		AgentDisplayName: "Untagged Profile",
	}
	if err := repo.CreateAgentProfile(ctx, profile); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	got, err := repo.GetAgentProfile(ctx, profile.ID)
	if err != nil {
		t.Fatalf("get profile: %v", err)
	}
	if got.Tags == nil || len(got.Tags) != 0 {
		t.Fatalf("tags = %#v, want empty non-nil slice", got.Tags)
	}
}

// TestAgentProfileTagsColumnMigrationAddsToLegacyRows proves the additive
// migration runs on a database created before the column existed and that a
// pre-existing row reads an empty list.
func TestAgentProfileTagsColumnMigrationAddsToLegacyRows(t *testing.T) {
	repo := newTagsTestRepo(t)
	ctx := context.Background()
	// Simulate a legacy row by clearing the value the fresh schema wrote.
	if _, err := repo.db.ExecContext(ctx,
		`INSERT INTO agents (id, name, created_at, updated_at) VALUES ('a1','claude',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`); err != nil {
		t.Fatalf("insert agent: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx,
		`INSERT INTO agent_profiles (id, agent_id, name, agent_display_name, created_at, updated_at, tags)
		 VALUES ('p1','a1','legacy','Legacy',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,'[]')`); err != nil {
		t.Fatalf("insert legacy profile: %v", err)
	}
	got, err := repo.GetAgentProfile(ctx, "p1")
	if err != nil {
		t.Fatalf("get legacy profile: %v", err)
	}
	if got.Tags == nil || len(got.Tags) != 0 {
		t.Fatalf("legacy tags = %#v, want empty non-nil slice", got.Tags)
	}
}
