package models

import (
	"fmt"
	"time"
)

// CanonicalDynamicPreferences canonicalizes a Dynamic Profile's soft
// preference lists and rejects a tag that appears in both. It is the single
// implementation shared by the settings controller and the workflow
// import/sync provisioner.
func CanonicalDynamicPreferences(preferred, avoided []string) ([]string, []string, error) {
	canonicalPreferred, err := CanonicalTags(preferred)
	if err != nil {
		return nil, nil, fmt.Errorf("preferred_tags: %w", err)
	}
	canonicalAvoided, err := CanonicalTags(avoided)
	if err != nil {
		return nil, nil, fmt.Errorf("avoided_tags: %w", err)
	}
	preferredSet := make(map[string]struct{}, len(canonicalPreferred))
	for _, tag := range canonicalPreferred {
		preferredSet[tag] = struct{}{}
	}
	for _, tag := range canonicalAvoided {
		if _, ok := preferredSet[tag]; ok {
			return nil, nil, fmt.Errorf("tag %q cannot be both preferred and avoided", tag)
		}
	}
	return canonicalPreferred, canonicalAvoided, nil
}

// DynamicAgentProfile stores the optimistic version for one dynamic profile's
// routing document. The parent agent_profiles row remains the profile's
// identity and display configuration.
type DynamicAgentProfile struct {
	ProfileID string    `json:"profile_id"`
	Version   int64     `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	// PreferredTags and AvoidedTags are soft scheduling preferences matched
	// against each candidate's concrete profile tags. They never discover
	// candidates and never override eligibility.
	PreferredTags []string `json:"preferred_tags"`
	AvoidedTags   []string `json:"avoided_tags"`
}

// DynamicAgentRoute is one ordered concrete candidate in a dynamic profile.
// RulesJSON is normalized by the dynamic-profile controller before persistence.
type DynamicAgentRoute struct {
	DynamicProfileID   string `json:"dynamic_profile_id"`
	Position           int    `json:"position"`
	ExecutionProfileID string `json:"execution_profile_id"`
	Enabled            bool   `json:"enabled"`
	RulesJSON          string `json:"rules_json"`
}

// DynamicProfileReference describes a dynamic profile that still points at a
// concrete profile. Soft-deleted parents are included so delete confirmation
// can explain stale configuration instead of silently losing the dependency.
type DynamicProfileReference struct {
	ProfileID string     `json:"profile_id"`
	Name      string     `json:"name"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
}
