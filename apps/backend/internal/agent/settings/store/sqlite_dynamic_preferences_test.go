package store

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/agent/settings/models"
)

func newDynamicPreferenceRepo(t *testing.T) *sqliteRepository {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo, err := newSQLiteRepository(db, db, nil, false)
	if err != nil {
		t.Fatalf("newSQLiteRepository: %v", err)
	}
	ctx := context.Background()
	if err := repo.CreateAgent(ctx, &models.Agent{ID: "dynamic", Name: "dynamic"}); err != nil {
		t.Fatalf("create dynamic family: %v", err)
	}
	for _, id := range []string{"profile-dynamic", "candidate-a"} {
		if err := repo.CreateAgentProfile(ctx, &models.AgentProfile{
			ID: id, AgentID: "dynamic", Name: id, AgentDisplayName: "Dynamic",
		}); err != nil {
			t.Fatalf("create profile %s: %v", id, err)
		}
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO agents (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)
	`, "concrete-agent", "concrete-agent", time.Now().UTC(), time.Now().UTC()); err != nil {
		t.Fatalf("create concrete family: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		UPDATE agent_profiles SET agent_id = ? WHERE id = ?
	`, "concrete-agent", "candidate-a"); err != nil {
		t.Fatalf("move candidate: %v", err)
	}
	return repo
}

func TestDynamicProfilePreferencesRoundTrip(t *testing.T) {
	repo := newDynamicPreferenceRepo(t)
	ctx := context.Background()

	created := &models.DynamicAgentProfile{
		ProfileID:     "profile-dynamic",
		Version:       1,
		PreferredTags: []string{"claude", "subscription"},
		AvoidedTags:   []string{"gemini"},
	}
	routes := []models.DynamicAgentRoute{
		{DynamicProfileID: created.ProfileID, Position: 0, ExecutionProfileID: "candidate-a", Enabled: true, RulesJSON: `{}`},
	}
	if err := repo.CreateDynamicAgentProfile(ctx, created, routes); err != nil {
		t.Fatalf("create dynamic config: %v", err)
	}

	got, _, err := repo.GetDynamicAgentProfile(ctx, created.ProfileID)
	if err != nil {
		t.Fatalf("get dynamic config: %v", err)
	}
	assertTagsEqual(t, "preferred", got.PreferredTags, []string{"claude", "subscription"})
	assertTagsEqual(t, "avoided", got.AvoidedTags, []string{"gemini"})

	updated := &models.DynamicAgentProfile{
		ProfileID:     created.ProfileID,
		PreferredTags: []string{"codex"},
		AvoidedTags:   []string{},
	}
	if err := repo.UpdateDynamicAgentProfile(ctx, updated, 1, routes); err != nil {
		t.Fatalf("update dynamic config: %v", err)
	}
	got, _, err = repo.GetDynamicAgentProfile(ctx, created.ProfileID)
	if err != nil {
		t.Fatalf("get updated dynamic config: %v", err)
	}
	assertTagsEqual(t, "preferred after update", got.PreferredTags, []string{"codex"})
	assertTagsEqual(t, "avoided after update", got.AvoidedTags, []string{})
}

func TestDynamicProfilePreferencesLegacyRowReadsEmpty(t *testing.T) {
	repo := newDynamicPreferenceRepo(t)
	ctx := context.Background()

	// A row written before the preference columns existed leaves them at the
	// column default. Read must normalize that to an empty, non-nil list.
	if _, err := repo.db.ExecContext(ctx, `
		INSERT INTO dynamic_agent_profiles (profile_id, version, created_at, updated_at)
		VALUES (?, ?, ?, ?)
	`, "profile-dynamic", 1, time.Now().UTC(), time.Now().UTC()); err != nil {
		t.Fatalf("insert legacy dynamic config: %v", err)
	}
	got, _, err := repo.GetDynamicAgentProfile(ctx, "profile-dynamic")
	if err != nil {
		t.Fatalf("get legacy dynamic config: %v", err)
	}
	assertTagsEqual(t, "legacy preferred", got.PreferredTags, []string{})
	assertTagsEqual(t, "legacy avoided", got.AvoidedTags, []string{})
}

func assertTagsEqual(t *testing.T, label string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s tags = %#v, want %#v", label, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s tags = %#v, want %#v", label, got, want)
		}
	}
}
