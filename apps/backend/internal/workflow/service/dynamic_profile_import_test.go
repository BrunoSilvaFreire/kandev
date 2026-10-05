package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/workflow/models"
)

// recordingProvisioner is an idempotent fake keyed on the migrated_from marker.
type recordingProvisioner struct {
	calls []string
	ids   map[string]string
}

func newRecordingProvisioner() *recordingProvisioner {
	return &recordingProvisioner{ids: map[string]string{}}
}

func (p *recordingProvisioner) EnsureGeneratedDynamicProfile(
	_ context.Context,
	migratedFrom, _ string,
	_ models.DynamicAgentProfilePortable,
) (string, error) {
	p.calls = append(p.calls, migratedFrom)
	if id, ok := p.ids[migratedFrom]; ok {
		return id, nil
	}
	id := fmt.Sprintf("generated-%d", len(p.ids)+1)
	p.ids[migratedFrom] = id
	return id, nil
}

func taggedClaudeExport() *models.WorkflowExport {
	return &models.WorkflowExport{
		Version: models.ExportVersion,
		Type:    models.ExportType,
		Workflows: []models.WorkflowPortable{{
			Name: "Review flow",
			Steps: []models.StepPortable{{
				Name:        "Review",
				Position:    0,
				Color:       "#aabbcc",
				AllowedTags: []string{"claude"},
				AgentProfile: &models.AgentProfilePortable{
					AgentName: "Claude",
				},
			}},
		}},
	}
}

func wireTaggedConversion(svc *Service, provisioner *recordingProvisioner) {
	svc.SetImportProfileCatalog(&importProfileCatalogFixture{profiles: []ImportProfileCandidate{{
		ID: "p-claude", Name: "Claude", AgentName: "Claude", Tags: []string{"claude"},
	}}})
	svc.SetAgentProfileFuncs(nil, func(string, string, string, string) string { return "" })
	svc.SetDynamicProfileProvisioner(provisioner)
}

func TestImportLegacyTaggedStepProvisionsGeneratedProfile(t *testing.T) {
	svc, _, _ := setupTestServiceWithProvider(t)
	provisioner := newRecordingProvisioner()
	wireTaggedConversion(svc, provisioner)

	result, err := svc.ImportWorkflowsWithBindings(context.Background(), "ws-1", taggedClaudeExport(), nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"Review flow"}, result.Created)
	assert.Equal(t, []string{"workflow_allowed_tags:ws-1:workflow:Review flow:step:0"}, provisioner.calls)

	steps, err := svc.repo.ListStepsByWorkflow(context.Background(), "imported-Review flow")
	require.NoError(t, err)
	require.Len(t, steps, 1)
	assert.Equal(t, "generated-1", steps[0].AgentProfileID)
	assert.Equal(t, []string{"claude"}, steps[0].AllowedTags)
}

func TestConvertLegacyTaggedExportRebindsExistingDynamicDescriptor(t *testing.T) {
	svc, _, _ := setupTestServiceWithProvider(t)
	provisioner := newRecordingProvisioner()
	wireTaggedConversion(svc, provisioner)
	ctx := context.Background()

	converted, bindings, err := svc.convertLegacyTaggedExport(ctx, "ws-1", taggedClaudeExport(), nil)
	require.NoError(t, err)
	require.Len(t, bindings, 1)
	require.NotNil(t, converted.Workflows[0].Steps[0].AgentProfile.Dynamic)

	// A document that already carries a dynamic descriptor is not re-converted,
	// but it is still provisioned so its step imports bound (B3).
	rebound, secondBindings, err := svc.convertLegacyTaggedExport(ctx, "ws-1", converted, nil)
	require.NoError(t, err)
	require.Len(t, secondBindings, 1)
	assert.Equal(t, bindings, secondBindings)
	assert.NotEmpty(t, rebound.Workflows[0].Steps[0].AgentProfile.Dynamic.Candidates[0].CandidateProfileID)
	assert.Len(t, provisioner.ids, 1, "the same generated profile was reused")
}

func TestApplySyncedWorkflowsLegacyTaggedConversionIsIdempotent(t *testing.T) {
	svc, provider, _ := setupSyncService(t)
	provisioner := newRecordingProvisioner()
	wireTaggedConversion(svc, provisioner)
	ctx := context.Background()
	files := []SyncFileExport{{Path: "workflows/review.yml", Export: taggedClaudeExport()}}

	first, err := svc.ApplySyncedWorkflows(ctx, "ws-1", files)
	require.NoError(t, err)
	assert.Equal(t, []string{"Review flow"}, first.Created)
	require.Len(t, provider.workflows, 1)

	second, err := svc.ApplySyncedWorkflows(ctx, "ws-1", files)
	require.NoError(t, err)
	assert.Empty(t, second.Created)
	assert.Empty(t, second.Updated)
	assert.Empty(t, second.Deleted)
	assert.Empty(t, second.Warnings)
	require.Len(t, provider.workflows, 1, "repeated sync created a duplicate workflow")

	// The same generated profile identity was reused on the second sync.
	require.Len(t, provisioner.calls, 2)
	assert.Equal(t, provisioner.calls[0], provisioner.calls[1])

	steps, err := svc.repo.ListStepsByWorkflow(ctx, provider.workflows[0].ID)
	require.NoError(t, err)
	require.Len(t, steps, 1)
	assert.Equal(t, provisioner.ids[provisioner.calls[0]], steps[0].AgentProfileID)
	assert.Equal(t, []string{"claude"}, steps[0].AllowedTags)
}

func userAuthoredDynamicExport() *models.WorkflowExport {
	return &models.WorkflowExport{
		Version: models.ExportVersion,
		Type:    models.ExportType,
		Workflows: []models.WorkflowPortable{{
			Name: "Authored flow",
			Steps: []models.StepPortable{{
				Name:     "Review",
				Position: 0,
				Color:    "#aabbcc",
				AgentProfile: &models.AgentProfilePortable{Dynamic: &models.DynamicAgentProfilePortable{
					PreferredTags: []string{"claude"},
					Candidates: []models.DynamicAgentCandidatePortable{{
						AgentProfile:       models.AgentProfilePortable{AgentName: "Claude"},
						CandidateProfileID: "p-claude",
						Enabled:            true,
					}},
				}},
			}},
		}},
	}
}

// B4: the preview must run on the converted view so it never asks for a
// binding that ImportWorkflowsWithBindings would reject as an unknown step.
func TestPreviewThenImportLegacyTaggedWorkflowSucceeds(t *testing.T) {
	svc, _, _ := setupTestServiceWithProvider(t)
	provisioner := newRecordingProvisioner()
	wireTaggedConversion(svc, provisioner)
	ctx := context.Background()

	preview, err := svc.PreviewImportWorkflows(ctx, "ws-1", taggedClaudeExport())
	require.NoError(t, err)
	assert.Empty(t, preview.Steps, "preview offered a binding for a provisioned step")

	// Submit exactly the preview's own steps as bindings (none for this export).
	bindings := make([]ImportProfileBinding, 0, len(preview.Steps))
	for _, step := range preview.Steps {
		requested := step.RequestedProfile
		bindings = append(bindings, ImportProfileBinding{
			WorkflowIndex: step.WorkflowIndex, StepPosition: step.StepPosition,
			RequestedProfile: &requested, ProfileID: "unused",
		})
	}
	result, err := svc.ImportWorkflowsWithBindings(ctx, "ws-1", taggedClaudeExport(), bindings)
	require.NoError(t, err)
	assert.Equal(t, []string{"Review flow"}, result.Created)

	steps, err := svc.repo.ListStepsByWorkflow(ctx, "imported-Review flow")
	require.NoError(t, err)
	require.Len(t, steps, 1)
	assert.Equal(t, "generated-1", steps[0].AgentProfileID)
}

func TestImportUserAuthoredDynamicStepBindsThroughImport(t *testing.T) {
	svc, _, _ := setupTestServiceWithProvider(t)
	provisioner := newRecordingProvisioner()
	wireTaggedConversion(svc, provisioner)

	result, err := svc.ImportWorkflowsWithBindings(context.Background(), "ws-1", userAuthoredDynamicExport(), nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"Authored flow"}, result.Created)

	steps, err := svc.repo.ListStepsByWorkflow(context.Background(), "imported-Authored flow")
	require.NoError(t, err)
	require.Len(t, steps, 1)
	assert.Equal(t, "generated-1", steps[0].AgentProfileID, "dynamic step imported unbound")
}

func TestImportUserAuthoredDynamicStepBindsThroughSync(t *testing.T) {
	svc, provider, _ := setupSyncService(t)
	provisioner := newRecordingProvisioner()
	wireTaggedConversion(svc, provisioner)

	result, err := svc.ApplySyncedWorkflows(context.Background(), "ws-1", []SyncFileExport{{
		Path: "workflows/authored.yml", Export: userAuthoredDynamicExport(),
	}})
	require.NoError(t, err)
	assert.Equal(t, []string{"Authored flow"}, result.Created)
	require.Len(t, provider.workflows, 1)

	steps, err := svc.repo.ListStepsByWorkflow(context.Background(), provider.workflows[0].ID)
	require.NoError(t, err)
	require.Len(t, steps, 1)
	assert.Equal(t, "generated-1", steps[0].AgentProfileID, "dynamic step imported unbound through sync")
}

func TestImportDynamicStepWithoutProvisionerStaysInteractive(t *testing.T) {
	svc, _, _ := setupTestServiceWithProvider(t)
	// No provisioner (dynamic routing disabled): the dynamic step must remain in
	// the interactive selection flow instead of being silently dropped.
	svc.SetImportProfileCatalog(&importProfileCatalogFixture{profiles: []ImportProfileCandidate{{
		ID: "p-claude", Name: "Claude", AgentName: "Claude", Tags: []string{"claude"},
	}}})
	svc.SetAgentProfileFuncs(nil, func(string, string, string, string) string { return "" })

	preview, err := svc.PreviewImportWorkflows(context.Background(), "ws-1", userAuthoredDynamicExport())
	require.NoError(t, err)
	require.Len(t, preview.Steps, 1, "dynamic step was dropped from interactive selection")
	assert.Equal(t, "Review", preview.Steps[0].StepName)
}

// S1: re-importing an existing workflow must not provision generated profiles
// for it, and must report it as skipped.
func TestReimportExistingWorkflowSkipsProvisioning(t *testing.T) {
	svc, _, _ := setupTestServiceWithProvider(t)
	provisioner := newRecordingProvisioner()
	wireTaggedConversion(svc, provisioner)
	ctx := context.Background()

	first, err := svc.ImportWorkflowsWithBindings(ctx, "ws-1", taggedClaudeExport(), nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"Review flow"}, first.Created)
	require.Len(t, provisioner.calls, 1)

	second, err := svc.ImportWorkflowsWithBindings(ctx, "ws-1", taggedClaudeExport(), nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"Review flow"}, second.Skipped)
	assert.Len(t, provisioner.calls, 1, "re-import provisioned a skipped workflow")

	legacy, err := svc.ImportWorkflows(ctx, "ws-1", taggedClaudeExport())
	require.NoError(t, err)
	assert.Equal(t, []string{"Review flow"}, legacy.Skipped)
	assert.Len(t, provisioner.calls, 1, "legacy re-import provisioned a skipped workflow")
}

func TestConvertLegacyTaggedExportNoCandidateFails(t *testing.T) {
	svc, _, _ := setupTestServiceWithProvider(t)
	provisioner := newRecordingProvisioner()
	svc.SetImportProfileCatalog(&importProfileCatalogFixture{profiles: []ImportProfileCandidate{{
		ID: "p-codex", Name: "Codex", AgentName: "Codex", Tags: []string{"codex"},
	}}})
	svc.SetAgentProfileFuncs(nil, func(string, string, string, string) string { return "" })
	svc.SetDynamicProfileProvisioner(provisioner)

	_, _, err := svc.convertLegacyTaggedExport(context.Background(), "ws-1", taggedClaudeExport(), nil)
	require.ErrorIs(t, err, models.ErrTaggedStepConversion)
	assert.Empty(t, provisioner.calls)
}
