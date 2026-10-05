package controller

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/agent/settings/store"
)

func TestValidateDynamicAgentProfilePreferences(t *testing.T) {
	base := func() *dto.DynamicAgentProfileDTO {
		return &dto.DynamicAgentProfileDTO{Candidates: []dto.DynamicAgentCandidateDTO{{
			Position: 0, ExecutionProfileID: "a", Rules: map[string]string{"on_provider_error": "try_next"},
		}}}
	}

	t.Run("canonicalizes and deduplicates", func(t *testing.T) {
		profile := base()
		profile.PreferredTags = []string{" Claude ", "claude", "Subscription"}
		profile.AvoidedTags = []string{"Gemini"}
		if err := validateDynamicAgentProfile(profile); err != nil {
			t.Fatalf("validate: %v", err)
		}
		wantPreferred := []string{"claude", "subscription"}
		if len(profile.PreferredTags) != len(wantPreferred) {
			t.Fatalf("preferred = %#v, want %#v", profile.PreferredTags, wantPreferred)
		}
		for i := range wantPreferred {
			if profile.PreferredTags[i] != wantPreferred[i] {
				t.Fatalf("preferred = %#v, want %#v", profile.PreferredTags, wantPreferred)
			}
		}
		if len(profile.AvoidedTags) != 1 || profile.AvoidedTags[0] != "gemini" {
			t.Fatalf("avoided = %#v", profile.AvoidedTags)
		}
	})

	t.Run("rejects overlap", func(t *testing.T) {
		profile := base()
		profile.PreferredTags = []string{"claude"}
		profile.AvoidedTags = []string{"Claude"}
		if err := validateDynamicAgentProfile(profile); !errors.Is(err, ErrInvalidDynamicPreferences) {
			t.Fatalf("error = %v, want %v", err, ErrInvalidDynamicPreferences)
		}
	})
}

func TestDynamicProfilePreferencesRoundTripThroughController(t *testing.T) {
	ctrl, repo := newSQLiteBackedController(t)
	if err := ctrl.agentRegistry.Register(agents.NewDynamicAgent()); err != nil {
		t.Fatalf("register dynamic agent: %v", err)
	}
	if err := ctrl.agentRegistry.Register(agents.NewClaudeACP()); err != nil {
		t.Fatalf("register concrete agent: %v", err)
	}
	ctrl.SetDynamicAgentRoutingEnabled(true)
	ctx := context.Background()
	if err := repo.CreateAgent(ctx, &models.Agent{ID: agents.DynamicAgentID, Name: agents.DynamicAgentID}); err != nil {
		t.Fatalf("create dynamic family: %v", err)
	}
	if err := repo.CreateAgent(ctx, &models.Agent{ID: "claude-family", Name: "claude-acp"}); err != nil {
		t.Fatalf("create concrete family: %v", err)
	}
	candidate := &models.AgentProfile{AgentID: "claude-family", Name: "Claude", AgentDisplayName: "Claude"}
	if err := repo.CreateAgentProfile(ctx, candidate); err != nil {
		t.Fatalf("create candidate: %v", err)
	}

	created, err := ctrl.CreateProfile(ctx, CreateProfileRequest{
		AgentID: agents.DynamicAgentID,
		Name:    "Review",
		Dynamic: &dto.DynamicAgentProfileDTO{
			PreferredTags: []string{"Claude", "subscription"},
			AvoidedTags:   []string{"gemini"},
			Candidates: []dto.DynamicAgentCandidateDTO{{
				Position: 0, ExecutionProfileID: candidate.ID, Enabled: true,
				Rules: map[string]string{"on_provider_error": "try_next"},
			}},
		},
	})
	if err != nil {
		t.Fatalf("create dynamic profile: %v", err)
	}
	if created.Dynamic == nil {
		t.Fatalf("created dynamic profile = %#v", created)
	}
	assertTagList(t, "created preferred", created.Dynamic.PreferredTags, []string{"claude", "subscription"})
	assertTagList(t, "created avoided", created.Dynamic.AvoidedTags, []string{"gemini"})

	dynamicRepo, ok := repo.(store.DynamicProfileRepository)
	if !ok {
		t.Fatalf("repo does not implement DynamicProfileRepository")
	}
	reloaded, _, err := dynamicRepo.GetDynamicAgentProfile(ctx, created.ID)
	if err != nil {
		t.Fatalf("reload dynamic profile: %v", err)
	}
	assertTagList(t, "stored preferred", reloaded.PreferredTags, []string{"claude", "subscription"})
	assertTagList(t, "stored avoided", reloaded.AvoidedTags, []string{"gemini"})

	if _, err := ctrl.CreateProfile(ctx, CreateProfileRequest{
		AgentID: agents.DynamicAgentID,
		Name:    "Conflicted",
		Dynamic: &dto.DynamicAgentProfileDTO{
			PreferredTags: []string{"claude"},
			AvoidedTags:   []string{"claude"},
			Candidates: []dto.DynamicAgentCandidateDTO{{
				Position: 0, ExecutionProfileID: candidate.ID, Enabled: true,
				Rules: map[string]string{"on_provider_error": "try_next"},
			}},
		},
	}); !errors.Is(err, ErrInvalidDynamicPreferences) {
		t.Fatalf("overlap create error = %v, want %v", err, ErrInvalidDynamicPreferences)
	}
}

func assertTagList(t *testing.T, label string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %#v, want %#v", label, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s = %#v, want %#v", label, got, want)
		}
	}
}
