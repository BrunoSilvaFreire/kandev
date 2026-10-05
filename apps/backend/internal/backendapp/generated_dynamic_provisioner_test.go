package backendapp

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/agents"
	agentsettingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
)

func newProvisionerTestRepos(t *testing.T) *Repositories {
	t.Helper()
	repos, _ := newMatcherTestRepos(t)
	ctx := context.Background()
	if err := repos.AgentSettings.CreateAgent(ctx, &agentsettingsmodels.Agent{ID: agents.DynamicAgentID, Name: agents.DynamicAgentID}); err != nil {
		t.Fatalf("create dynamic family: %v", err)
	}
	return repos
}

func newProvisionerFixture(t *testing.T) (*generatedDynamicProvisioner, *Repositories) {
	t.Helper()
	repos := newProvisionerTestRepos(t)
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	return newGeneratedDynamicProvisioner(repos, log), repos
}

func createConcreteTaggedProfile(
	t *testing.T,
	repos *Repositories,
	agentID, displayName string,
	tags []string,
) *agentsettingsmodels.AgentProfile {
	t.Helper()
	profile := &agentsettingsmodels.AgentProfile{
		AgentID:          agentID,
		Name:             displayName + " " + uuid.NewString(),
		AgentDisplayName: displayName,
		Model:            "sonnet",
		Enabled:          true,
		Tags:             tags,
	}
	if err := repos.AgentSettings.CreateAgentProfile(context.Background(), profile); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	return profile
}

func descriptorFor(candidateID, agentName string, preferred []string) workflowmodels.DynamicAgentProfilePortable {
	return workflowmodels.DynamicAgentProfilePortable{
		PreferredTags: preferred,
		Candidates: []workflowmodels.DynamicAgentCandidatePortable{{
			AgentProfile:       workflowmodels.AgentProfilePortable{AgentName: agentName, Model: "sonnet"},
			CandidateProfileID: candidateID,
			Enabled:            true,
		}},
	}
}

func storedRouteIDs(t *testing.T, repos *Repositories, profileID string) []string {
	t.Helper()
	dynamicRepo, ok := repos.AgentSettings.(settingsstore.DynamicProfileRepository)
	require.True(t, ok)
	_, routes, err := dynamicRepo.GetDynamicAgentProfile(context.Background(), profileID)
	require.NoError(t, err)
	ids := make([]string, 0, len(routes))
	for _, route := range routes {
		ids = append(ids, route.ExecutionProfileID)
	}
	return ids
}

// B1: two profiles sharing an agent/model/mode but told apart by tags must stay
// distinct; the carried candidate ID wins over the oldest-triple matcher.
func TestGeneratedProvisionerKeepsTagDistinguishedCandidate(t *testing.T) {
	provisioner, repos := newProvisionerFixture(t)
	agentID := createMatcherTestAgent(t, repos)
	older := createConcreteTaggedProfile(t, repos, agentID, "Claude", []string{"work"})
	newer := createConcreteTaggedProfile(t, repos, agentID, "Claude", []string{"personal"})

	ctx := context.Background()
	marker := "workflow_allowed_tags:ws-1:workflow:Review flow:step:0"
	id, err := provisioner.EnsureGeneratedDynamicProfile(ctx, marker, "Review flow / Review", descriptorFor(newer.ID, "Claude", []string{"personal"}))
	require.NoError(t, err)

	routes := storedRouteIDs(t, repos, id)
	require.Len(t, routes, 1)
	assert.Equal(t, newer.ID, routes[0])
	assert.NotEqual(t, older.ID, routes[0])
}

// B2.1: identical workflow/step names in different workspaces get distinct
// generated profiles with their own candidates.
func TestGeneratedProvisionerScopesIdentityByWorkspace(t *testing.T) {
	provisioner, repos := newProvisionerFixture(t)
	agentID := createMatcherTestAgent(t, repos)
	claude := createConcreteTaggedProfile(t, repos, agentID, "Claude", []string{"claude"})
	codex := createConcreteTaggedProfile(t, repos, agentID, "Codex", []string{"codex"})
	ctx := context.Background()

	idA, err := provisioner.EnsureGeneratedDynamicProfile(ctx, "workflow_allowed_tags:ws-a:workflow:Review:step:0", "Review / B", descriptorFor(claude.ID, "Claude", []string{"claude"}))
	require.NoError(t, err)
	idB, err := provisioner.EnsureGeneratedDynamicProfile(ctx, "workflow_allowed_tags:ws-b:workflow:Review:step:0", "Review / B", descriptorFor(codex.ID, "Codex", []string{"codex"}))
	require.NoError(t, err)

	assert.NotEqual(t, idA, idB)
	assert.Equal(t, []string{claude.ID}, storedRouteIDs(t, repos, idA))
	assert.Equal(t, []string{codex.ID}, storedRouteIDs(t, repos, idB))
}

// B2.2: a marker hit whose tag preferences and candidates changed updates the
// generated profile instead of silently reusing the stale one.
func TestGeneratedProvisionerReconcilesChangedContent(t *testing.T) {
	provisioner, repos := newProvisionerFixture(t)
	agentID := createMatcherTestAgent(t, repos)
	claude := createConcreteTaggedProfile(t, repos, agentID, "Claude", []string{"claude"})
	codex := createConcreteTaggedProfile(t, repos, agentID, "Codex", []string{"codex"})
	ctx := context.Background()
	marker := "workflow_allowed_tags:ws-1:workflow:Review:step:0"

	first, err := provisioner.EnsureGeneratedDynamicProfile(ctx, marker, "Review / Review", descriptorFor(claude.ID, "Claude", []string{"claude"}))
	require.NoError(t, err)
	updated, err := provisioner.EnsureGeneratedDynamicProfile(ctx, marker, "Review / Review", descriptorFor(codex.ID, "Codex", []string{"codex"}))
	require.NoError(t, err)
	assert.Equal(t, first, updated)

	dynamicRepo, ok := repos.AgentSettings.(settingsstore.DynamicProfileRepository)
	require.True(t, ok)
	config, _, err := dynamicRepo.GetDynamicAgentProfile(ctx, first)
	require.NoError(t, err)
	assert.Equal(t, []string{"codex"}, config.PreferredTags)
	assert.Equal(t, []string{codex.ID}, storedRouteIDs(t, repos, first))
}

// S7: a user disabling a generated candidate is preserved across a reconcile
// and does not count as content drift.
func TestGeneratedProvisionerPreservesDisabledCandidate(t *testing.T) {
	provisioner, repos := newProvisionerFixture(t)
	agentID := createMatcherTestAgent(t, repos)
	claude := createConcreteTaggedProfile(t, repos, agentID, "Claude", []string{"claude"})
	ctx := context.Background()
	marker := "workflow_allowed_tags:ws:workflow:W:step:0"
	descriptor := descriptorFor(claude.ID, "Claude", []string{"claude"})

	id, err := provisioner.EnsureGeneratedDynamicProfile(ctx, marker, "W / Review", descriptor)
	require.NoError(t, err)
	dynamicRepo, ok := repos.AgentSettings.(settingsstore.DynamicProfileRepository)
	require.True(t, ok)
	config, routes, err := dynamicRepo.GetDynamicAgentProfile(ctx, id)
	require.NoError(t, err)
	require.Len(t, routes, 1)
	routes[0].Enabled = false
	require.NoError(t, dynamicRepo.UpdateDynamicAgentProfile(ctx, &agentsettingsmodels.DynamicAgentProfile{
		ProfileID: id, PreferredTags: config.PreferredTags, AvoidedTags: config.AvoidedTags,
	}, config.Version, routes))
	disabledConfig, disabledRoutes, err := dynamicRepo.GetDynamicAgentProfile(ctx, id)
	require.NoError(t, err)
	require.False(t, disabledRoutes[0].Enabled)
	version := disabledConfig.Version

	again, err := provisioner.EnsureGeneratedDynamicProfile(ctx, marker, "W / Review", descriptor)
	require.NoError(t, err)
	assert.Equal(t, id, again)
	finalConfig, finalRoutes, err := dynamicRepo.GetDynamicAgentProfile(ctx, id)
	require.NoError(t, err)
	assert.False(t, finalRoutes[0].Enabled, "disabled candidate was re-enabled")
	assert.Equal(t, version, finalConfig.Version, "disabled candidate counted as drift")
}

// B5: a non-canonical descriptor is stored canonical and does not look like
// drift on a repeated call.
func TestGeneratedProvisionerCanonicalizesDescriptor(t *testing.T) {
	provisioner, repos := newProvisionerFixture(t)
	agentID := createMatcherTestAgent(t, repos)
	claude := createConcreteTaggedProfile(t, repos, agentID, "Claude", []string{"claude"})
	ctx := context.Background()
	descriptor := descriptorFor(claude.ID, "Claude", []string{" Claude ", "claude"})

	id, err := provisioner.EnsureGeneratedDynamicProfile(ctx, "workflow_allowed_tags:ws:workflow:W:step:0", "W / Review", descriptor)
	require.NoError(t, err)
	dynamicRepo, ok := repos.AgentSettings.(settingsstore.DynamicProfileRepository)
	require.True(t, ok)
	config, _, err := dynamicRepo.GetDynamicAgentProfile(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, []string{"claude"}, config.PreferredTags)

	version := config.Version
	again, err := provisioner.EnsureGeneratedDynamicProfile(ctx, "workflow_allowed_tags:ws:workflow:W:step:0", "W / Review", descriptor)
	require.NoError(t, err)
	assert.Equal(t, id, again)
	reloaded, _, err := dynamicRepo.GetDynamicAgentProfile(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, version, reloaded.Version, "canonical descriptor was treated as drift")
}

// B5: a descriptor whose preferred and avoided lists overlap is rejected.
func TestGeneratedProvisionerRejectsOverlappingPreferences(t *testing.T) {
	provisioner, repos := newProvisionerFixture(t)
	agentID := createMatcherTestAgent(t, repos)
	claude := createConcreteTaggedProfile(t, repos, agentID, "Claude", []string{"claude"})
	descriptor := descriptorFor(claude.ID, "Claude", []string{"claude"})
	descriptor.AvoidedTags = []string{"claude"}

	_, err := provisioner.EnsureGeneratedDynamicProfile(context.Background(), "workflow_allowed_tags:ws:workflow:W:step:0", "W / Review", descriptor)
	require.Error(t, err)
}

// B3: a user-authored dynamic descriptor (no marker) reuses an existing profile
// with the same content instead of creating a duplicate.
func TestGeneratedProvisionerReusesContentMatchedProfile(t *testing.T) {
	provisioner, repos := newProvisionerFixture(t)
	agentID := createMatcherTestAgent(t, repos)
	claude := createConcreteTaggedProfile(t, repos, agentID, "Claude", []string{"claude"})
	ctx := context.Background()
	descriptor := descriptorFor(claude.ID, "Claude", []string{"claude"})

	first, err := provisioner.EnsureGeneratedDynamicProfile(ctx, "", "Imported", descriptor)
	require.NoError(t, err)
	second, err := provisioner.EnsureGeneratedDynamicProfile(ctx, "", "Imported", descriptor)
	require.NoError(t, err)
	assert.Equal(t, first, second, "content-identical descriptor created a duplicate profile")
}
