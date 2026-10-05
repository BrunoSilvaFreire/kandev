package backendapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/runtime/routingpolicy"
	agentsettingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
)

// generatedDynamicProvisioner materializes the Dynamic Profile described by an
// incoming portable descriptor. It resolves candidates by their source profile
// ID first (so tag-distinguished profiles that share an agent/model/mode are
// kept distinct), reconciles content on a provenance-marker hit, and matches a
// user-authored descriptor by content. It is therefore idempotent across
// repeated imports and syncs.
type generatedDynamicProvisioner struct {
	repo     settingsstore.Repository
	dynamic  settingsstore.DynamicProfileRepository
	match    workflowmodels.AgentProfileMatcher
	policies json.RawMessage
	log      *logger.Logger
}

func newGeneratedDynamicProvisioner(repos *Repositories, log *logger.Logger) *generatedDynamicProvisioner {
	policies, err := json.Marshal(routingpolicy.DefaultDocument())
	if err != nil {
		policies = []byte(`{}`)
	}
	provisioner := &generatedDynamicProvisioner{
		repo:     repos.AgentSettings,
		match:    buildAgentProfileMatcher(repos, log),
		policies: policies,
		log:      log,
	}
	if dynamicRepo, ok := repos.AgentSettings.(settingsstore.DynamicProfileRepository); ok {
		provisioner.dynamic = dynamicRepo
	}
	return provisioner
}

// EnsureGeneratedDynamicProfile implements workflowservice.DynamicProfileProvisioner.
func (p *generatedDynamicProvisioner) EnsureGeneratedDynamicProfile(
	ctx context.Context,
	migratedFrom, displayName string,
	descriptor workflowmodels.DynamicAgentProfilePortable,
) (string, error) {
	if p == nil || p.repo == nil || p.dynamic == nil {
		return "", errors.New("generated dynamic profile provisioning is not configured")
	}
	preferred, avoided, err := agentsettingsmodels.CanonicalDynamicPreferences(descriptor.PreferredTags, descriptor.AvoidedTags)
	if err != nil {
		return "", fmt.Errorf("invalid dynamic profile %q preferences: %w", displayName, err)
	}
	descriptor.PreferredTags = preferred
	descriptor.AvoidedTags = avoided
	routes, err := p.routesForDescriptor(ctx, descriptor)
	if err != nil {
		return "", err
	}
	if len(routes) == 0 {
		return "", fmt.Errorf("dynamic profile %q resolved no concrete candidates", displayName)
	}
	if migratedFrom != "" {
		existing, findErr := p.findByMigratedFrom(ctx, migratedFrom)
		if findErr != nil {
			return "", findErr
		}
		if existing != "" {
			return p.reconcile(ctx, existing, descriptor, routes)
		}
		return p.create(ctx, migratedFrom, displayName, descriptor, routes)
	}
	existing, findErr := p.findMatchingDynamicProfile(ctx, descriptor, routes)
	if findErr != nil {
		return "", findErr
	}
	if existing != "" {
		return existing, nil
	}
	return p.create(ctx, "", displayName, descriptor, routes)
}

func (p *generatedDynamicProvisioner) reconcile(
	ctx context.Context,
	profileID string,
	descriptor workflowmodels.DynamicAgentProfilePortable,
	desired []agentsettingsmodels.DynamicAgentRoute,
) (string, error) {
	config, routes, err := p.dynamic.GetDynamicAgentProfile(ctx, profileID)
	if err != nil {
		return "", fmt.Errorf("load generated dynamic profile %s: %w", profileID, err)
	}
	if dynamicProfileMatches(config, routes, descriptor, desired) {
		return profileID, nil
	}
	// Preserve a user-edited per-candidate policy and enabled choice for a
	// candidate that survives the change; only new candidates get the canonical
	// defaults.
	existingPolicy := make(map[string]string, len(routes))
	existingEnabled := make(map[string]bool, len(routes))
	for _, route := range routes {
		if route.RulesJSON != "" {
			existingPolicy[route.ExecutionProfileID] = route.RulesJSON
		}
		existingEnabled[route.ExecutionProfileID] = route.Enabled
	}
	for index := range desired {
		if policy, ok := existingPolicy[desired[index].ExecutionProfileID]; ok {
			desired[index].RulesJSON = policy
		}
		if enabled, ok := existingEnabled[desired[index].ExecutionProfileID]; ok {
			desired[index].Enabled = enabled
		}
	}
	updated := &agentsettingsmodels.DynamicAgentProfile{
		ProfileID:     profileID,
		PreferredTags: descriptor.PreferredTags,
		AvoidedTags:   descriptor.AvoidedTags,
	}
	if err := p.dynamic.UpdateDynamicAgentProfile(ctx, updated, config.Version, desired); err != nil {
		return "", fmt.Errorf("update generated dynamic profile %s: %w", profileID, err)
	}
	return profileID, nil
}

func (p *generatedDynamicProvisioner) create(
	ctx context.Context,
	migratedFrom, displayName string,
	descriptor workflowmodels.DynamicAgentProfilePortable,
	routes []agentsettingsmodels.DynamicAgentRoute,
) (string, error) {
	profile := &agentsettingsmodels.AgentProfile{
		ID:               uuid.NewString(),
		AgentID:          agents.DynamicAgentID,
		Name:             generatedProfileName(displayName, migratedFrom),
		AgentDisplayName: agents.DynamicAgentID,
		Enabled:          true,
		MigratedFrom:     migratedFrom,
		UserModified:     true,
		CLIFlags:         []agentsettingsmodels.CLIFlag{},
		EnvVars:          []agentsettingsmodels.ProfileEnvVar{},
	}
	if err := p.repo.CreateAgentProfile(ctx, profile); err != nil {
		return "", fmt.Errorf("create dynamic profile %s: %w", migratedFrom, err)
	}
	dynamic := &agentsettingsmodels.DynamicAgentProfile{
		ProfileID:     profile.ID,
		Version:       1,
		PreferredTags: descriptor.PreferredTags,
		AvoidedTags:   descriptor.AvoidedTags,
	}
	if err := p.dynamic.CreateDynamicAgentProfile(ctx, dynamic, routes); err != nil {
		if cleanupErr := p.repo.DeleteAgentProfile(ctx, profile.ID); cleanupErr != nil {
			return "", fmt.Errorf("%w; cleanup dynamic profile: %v", err, cleanupErr)
		}
		return "", err
	}
	return profile.ID, nil
}

func (p *generatedDynamicProvisioner) findByMigratedFrom(ctx context.Context, migratedFrom string) (string, error) {
	profiles, err := p.listDynamicProfiles(ctx)
	if err != nil {
		return "", err
	}
	for _, profile := range profiles {
		if profile.MigratedFrom == migratedFrom {
			return profile.ID, nil
		}
	}
	return "", nil
}

// findMatchingDynamicProfile returns an existing Dynamic Profile whose
// preferences and ordered candidates already equal the descriptor, so a
// repeated user-authored export/import reuses one profile instead of creating a
// duplicate.
func (p *generatedDynamicProvisioner) findMatchingDynamicProfile(
	ctx context.Context,
	descriptor workflowmodels.DynamicAgentProfilePortable,
	desired []agentsettingsmodels.DynamicAgentRoute,
) (string, error) {
	profiles, err := p.listDynamicProfiles(ctx)
	if err != nil {
		return "", err
	}
	for _, profile := range profiles {
		// Only a user-authored profile (no marker) may match by content; a
		// generated profile belongs to the step its marker names.
		if profile.MigratedFrom != "" {
			continue
		}
		config, routes, configErr := p.dynamic.GetDynamicAgentProfile(ctx, profile.ID)
		if configErr != nil {
			continue
		}
		if dynamicProfileMatches(config, routes, descriptor, desired) {
			return profile.ID, nil
		}
	}
	return "", nil
}

func (p *generatedDynamicProvisioner) listDynamicProfiles(ctx context.Context) ([]*agentsettingsmodels.AgentProfile, error) {
	agentsList, err := p.repo.ListAgents(ctx)
	if err != nil {
		return nil, err
	}
	profiles := make([]*agentsettingsmodels.AgentProfile, 0)
	for _, agent := range agentsList {
		if agent.ID != agents.DynamicAgentID {
			continue
		}
		list, listErr := p.repo.ListAgentProfiles(ctx, agent.ID)
		if listErr != nil {
			return nil, listErr
		}
		profiles = append(profiles, list...)
	}
	return profiles, nil
}

// routesForDescriptor resolves each candidate to a concrete local profile,
// preferring the carried source profile ID so two profiles that share an
// agent/model/mode but differ by tags stay distinct.
func (p *generatedDynamicProvisioner) routesForDescriptor(
	ctx context.Context,
	descriptor workflowmodels.DynamicAgentProfilePortable,
) ([]agentsettingsmodels.DynamicAgentRoute, error) {
	routes := make([]agentsettingsmodels.DynamicAgentRoute, 0, len(descriptor.Candidates))
	for _, candidate := range descriptor.Candidates {
		profileID := p.resolveCandidateProfileID(ctx, candidate)
		if profileID == "" {
			if p.log != nil {
				p.log.Warn("dynamic profile candidate could not be resolved during import",
					zap.String("agent_name", candidate.AgentProfile.AgentName),
					zap.String("model", candidate.AgentProfile.Model))
			}
			continue
		}
		routes = append(routes, agentsettingsmodels.DynamicAgentRoute{
			Position:           len(routes),
			ExecutionProfileID: profileID,
			Enabled:            candidate.Enabled,
			RulesJSON:          string(p.policies),
		})
	}
	return routes, nil
}

func (p *generatedDynamicProvisioner) resolveCandidateProfileID(
	ctx context.Context,
	candidate workflowmodels.DynamicAgentCandidatePortable,
) string {
	if candidate.CandidateProfileID != "" {
		profile, err := p.repo.GetAgentProfile(ctx, candidate.CandidateProfileID)
		if err == nil && isEligibleGeneratedCandidate(profile) {
			return profile.ID
		}
	}
	if p.match == nil {
		return ""
	}
	return p.match(candidate.AgentProfile.AgentName, candidate.AgentProfile.Model, candidate.AgentProfile.Mode, "")
}

func isEligibleGeneratedCandidate(profile *agentsettingsmodels.AgentProfile) bool {
	return profile != nil && profile.ID != "" && profile.Enabled && profile.WorkspaceID == "" &&
		profile.DeletedAt == nil && profile.AgentID != agents.DynamicAgentID
}

func dynamicProfileMatches(
	config *agentsettingsmodels.DynamicAgentProfile,
	routes []agentsettingsmodels.DynamicAgentRoute,
	descriptor workflowmodels.DynamicAgentProfilePortable,
	desired []agentsettingsmodels.DynamicAgentRoute,
) bool {
	if config == nil {
		return false
	}
	if !equalTagLists(config.PreferredTags, descriptor.PreferredTags) ||
		!equalTagLists(config.AvoidedTags, descriptor.AvoidedTags) {
		return false
	}
	if len(routes) != len(desired) {
		return false
	}
	for index := range routes {
		if routes[index].ExecutionProfileID != desired[index].ExecutionProfileID {
			return false
		}
	}
	// Enabled is intentionally not compared: a user disabling a candidate is a
	// preference, not drift, and is preserved by reconcile.
	return true
}

// equalTagLists treats a nil and an empty list as equal so a stored "[]"
// column does not look like drift from a descriptor with no avoided tags.
func equalTagLists(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for index := range a {
		if a[index] != b[index] {
			return false
		}
	}
	return true
}

func generatedProfileName(displayName, migratedFrom string) string {
	if displayName != "" {
		return displayName
	}
	if migratedFrom != "" {
		return migratedFrom
	}
	return "Imported dynamic profile"
}
