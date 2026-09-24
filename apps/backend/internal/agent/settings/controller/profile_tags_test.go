package controller

import (
	"context"
	"errors"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
)

func newTagsController(t *testing.T) (*Controller, store.Repository) {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo, cleanup, err := store.Provide(db, db, log)
	if err != nil {
		t.Fatalf("settings store: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })

	reg := registry.NewRegistry(log)
	ag := &testAgent{id: "test-agent", name: "test-agent", displayName: "Test Agent", enabled: true}
	if err := reg.Register(ag); err != nil {
		t.Fatalf("register agent: %v", err)
	}
	return NewController(repo, nil, reg, nil, log), repo
}

func seedTagsAgent(t *testing.T, repo store.Repository) {
	t.Helper()
	if err := repo.CreateAgent(context.Background(), &models.Agent{ID: "test-agent", Name: "test-agent"}); err != nil {
		t.Fatalf("create agent row: %v", err)
	}
}

func TestCreateProfileCanonicalizesTags(t *testing.T) {
	ctrl, repo := newTagsController(t)
	seedTagsAgent(t, repo)
	ctx := context.Background()

	result, err := ctrl.CreateProfile(ctx, CreateProfileRequest{
		AgentID: "test-agent",
		Name:    "Tagged",
		Tags:    []string{" Review ", "SECURITY", "review"},
	})
	if err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if len(result.Tags) != 2 || result.Tags[0] != "review" || result.Tags[1] != "security" {
		t.Fatalf("tags = %#v, want [review security]", result.Tags)
	}

	stored, err := repo.GetAgentProfile(ctx, result.ID)
	if err != nil {
		t.Fatalf("get profile: %v", err)
	}
	if len(stored.Tags) != 2 || stored.Tags[0] != "review" {
		t.Fatalf("stored tags = %#v", stored.Tags)
	}
}

func TestCreateProfileRejectsInvalidTags(t *testing.T) {
	ctrl, repo := newTagsController(t)
	seedTagsAgent(t, repo)
	_, err := ctrl.CreateProfile(context.Background(), CreateProfileRequest{
		AgentID: "test-agent",
		Name:    "Bad",
		Tags:    []string{"ok", "  "},
	})
	if !errors.Is(err, ErrInvalidProfileTags) {
		t.Fatalf("err = %v, want ErrInvalidProfileTags", err)
	}
}

func TestUpdateProfileCanonicalizesAndRejectsTags(t *testing.T) {
	ctrl, repo := newTagsController(t)
	ctx := context.Background()
	profile := &models.AgentProfile{AgentID: "test-agent", Name: "P", Model: "m"}
	if err := repo.CreateAgentProfile(ctx, profile); err != nil {
		t.Fatalf("seed profile: %v", err)
	}

	updated, err := ctrl.UpdateProfile(ctx, UpdateProfileRequest{
		ID:   profile.ID,
		Tags: &[]string{"Ops", "ops", " review "},
	})
	if err != nil {
		t.Fatalf("update profile: %v", err)
	}
	if len(updated.Tags) != 2 || updated.Tags[0] != "ops" || updated.Tags[1] != "review" {
		t.Fatalf("tags = %#v, want [ops review]", updated.Tags)
	}

	_, err = ctrl.UpdateProfile(ctx, UpdateProfileRequest{
		ID:   profile.ID,
		Tags: &[]string{""},
	})
	if !errors.Is(err, ErrInvalidProfileTags) {
		t.Fatalf("err = %v, want ErrInvalidProfileTags", err)
	}
}

func TestDuplicateProfileCopiesTags(t *testing.T) {
	ctrl, repo := newTagsController(t)
	ctx := context.Background()
	profile := &models.AgentProfile{
		AgentID: "test-agent", Name: "P", Model: "m", Tags: []string{"review"},
	}
	if err := repo.CreateAgentProfile(ctx, profile); err != nil {
		t.Fatalf("seed profile: %v", err)
	}
	copyDTO, err := ctrl.DuplicateProfile(ctx, DuplicateProfileRequest{ID: profile.ID})
	if err != nil {
		t.Fatalf("duplicate: %v", err)
	}
	if len(copyDTO.Tags) != 1 || copyDTO.Tags[0] != "review" {
		t.Fatalf("copy tags = %#v, want [review]", copyDTO.Tags)
	}
}

func TestDynamicProfileRejectsTags(t *testing.T) {
	ctrl, repo := newTagsController(t)
	ctrl.dynamicAgentRoutingEnabled = true
	ctx := context.Background()
	profile := &models.AgentProfile{AgentID: agents.DynamicAgentID, Name: "D"}
	if err := repo.CreateAgentProfile(ctx, profile); err != nil {
		t.Fatalf("seed dynamic profile: %v", err)
	}
	_, err := ctrl.UpdateProfile(ctx, UpdateProfileRequest{
		ID:   profile.ID,
		Tags: &[]string{"review"},
	})
	if !errors.Is(err, ErrInvalidProfileTags) {
		t.Fatalf("err = %v, want ErrInvalidProfileTags", err)
	}
}
