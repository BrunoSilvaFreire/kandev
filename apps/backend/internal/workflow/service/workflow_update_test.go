package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/workflow/models"
)

func TestWorkflowUpdatePreviewAndApplyPreservesWorkflowAndStepIdentity(t *testing.T) {
	svc, database := setupTestService(t)
	ctx := context.Background()
	insertWorkflow(t, database, "wf-update", "Original")
	_, err := database.Exec(`UPDATE workflows SET workspace_id = 'ws-update' WHERE id = 'wf-update'`)
	require.NoError(t, err)
	provider := &mockWorkflowProvider{}
	provider.addWorkflow("wf-update", "ws-update", "Original")
	svc.SetWorkflowProvider(provider)
	createStep(t, svc, &models.WorkflowStep{ID: "stable-step", WorkflowID: "wf-update", PortableKey: "stable-key", Name: "Queued", Position: 0, IsStartStep: true})
	definition := portableWorkflow("Updated", "Ready")
	definition.Steps[0].Key = "stable-key"
	definition.Prompt = "new workflow instructions"
	export := exportOf(definition)
	preview, err := svc.PreviewWorkflowUpdate(ctx, "ws-update", "wf-update", export, WorkflowUpdateRequest{WorkflowIndex: intPointer(0)})
	require.NoError(t, err)
	require.True(t, preview.CanApply)
	require.NotEmpty(t, preview.Version)
	require.NotEmpty(t, preview.Changes)
	require.NoError(t, svc.ApplyWorkflowUpdate(ctx, "ws-update", "wf-update", preview.Version, export, WorkflowUpdateRequest{WorkflowIndex: intPointer(0)}))
	step, err := svc.repo.GetStep(ctx, "stable-step")
	require.NoError(t, err)
	require.Equal(t, "Ready", step.Name)
	var workflowID string
	require.NoError(t, database.Get(&workflowID, `SELECT id FROM workflows WHERE name = 'Updated'`))
	require.Equal(t, "wf-update", workflowID)
}

func TestWorkflowUpdateRejectsStalePreviewToken(t *testing.T) {
	svc, database := setupTestService(t)
	ctx := context.Background()
	insertWorkflow(t, database, "wf-stale-update", "Original")
	_, err := database.Exec(`UPDATE workflows SET workspace_id = 'ws-update' WHERE id = 'wf-stale-update'`)
	require.NoError(t, err)
	provider := &mockWorkflowProvider{}
	provider.addWorkflow("wf-stale-update", "ws-update", "Original")
	svc.SetWorkflowProvider(provider)
	createStep(t, svc, &models.WorkflowStep{ID: "stale-step", WorkflowID: "wf-stale-update", PortableKey: "stale-key", Name: "Queued", Position: 0, IsStartStep: true})
	definition := portableWorkflow("Updated", "Ready")
	definition.Steps[0].Key = "stale-key"
	export := exportOf(definition)
	preview, err := svc.PreviewWorkflowUpdate(ctx, "ws-update", "wf-stale-update", export, WorkflowUpdateRequest{WorkflowIndex: intPointer(0)})
	require.NoError(t, err)
	_, err = database.Exec(`UPDATE workflows SET prompt = 'changed after preview' WHERE id = 'wf-stale-update'`)
	require.NoError(t, err)
	err = svc.ApplyWorkflowUpdate(ctx, "ws-update", "wf-stale-update", preview.Version, export, WorkflowUpdateRequest{WorkflowIndex: intPointer(0)})
	require.ErrorIs(t, err, repoerrors.ErrTaskVersionConflict)
	step, err := svc.repo.GetStep(ctx, "stale-step")
	require.NoError(t, err)
	require.Equal(t, "Queued", step.Name)
}

func TestWorkflowUpdateRequiresMappingForUnkeyedRename(t *testing.T) {
	svc, database := setupTestService(t)
	ctx := context.Background()
	insertWorkflow(t, database, "wf-legacy-update", "Original")
	_, err := database.Exec(`UPDATE workflows SET workspace_id = 'ws-update' WHERE id = 'wf-legacy-update'`)
	require.NoError(t, err)
	provider := &mockWorkflowProvider{workflows: []*taskmodels.Workflow{{ID: "wf-legacy-update", WorkspaceID: "ws-update", Name: "Original"}}}
	svc.SetWorkflowProvider(provider)
	createStep(t, svc, &models.WorkflowStep{ID: "legacy-step", WorkflowID: "wf-legacy-update", Name: "Queued", Position: 0, IsStartStep: true})
	preview, err := svc.PreviewWorkflowUpdate(ctx, "ws-update", "wf-legacy-update", exportOf(portableWorkflow("Original", "Ready")), WorkflowUpdateRequest{WorkflowIndex: intPointer(0)})
	require.NoError(t, err)
	require.False(t, preview.CanApply)
	require.Contains(t, preview.Errors, "step \"Ready\" could be a rename; provide a stable key or old step ID to new name mapping")
}

func TestWorkflowUpdateDoesNotReuseRetiredPortableKey(t *testing.T) {
	svc, database := setupTestService(t)
	ctx := context.Background()
	insertWorkflow(t, database, "wf-retired-key", "Original")
	_, err := database.Exec(`UPDATE workflows SET workspace_id = 'ws-update' WHERE id = 'wf-retired-key'`)
	require.NoError(t, err)
	provider := &mockWorkflowProvider{workflows: []*taskmodels.Workflow{{ID: "wf-retired-key", WorkspaceID: "ws-update", Name: "Original"}}}
	svc.SetWorkflowProvider(provider)
	createStep(t, svc, &models.WorkflowStep{ID: "retired-key-step", WorkflowID: "wf-retired-key", PortableKey: "retired-key", Name: "Old", Position: 0, IsStartStep: true})
	require.NoError(t, svc.DeleteStep(ctx, "retired-key-step"))
	definition := portableWorkflow("Original", "New")
	definition.Steps[0].Key = "retired-key"
	preview, err := svc.PreviewWorkflowUpdate(ctx, "ws-update", "wf-retired-key", exportOf(definition), WorkflowUpdateRequest{WorkflowIndex: intPointer(0)})
	require.NoError(t, err)
	require.False(t, preview.CanApply)
	require.Contains(t, preview.Errors, `portable step key "retired-key" belongs to a retired historical step and cannot be reused`)
}

func intPointer(value int) *int { return &value }

func TestWorkflowUpdateRequiresExplicitSelectionForMultipleDefinitions(t *testing.T) {
	svc, database := setupTestService(t)
	ctx := context.Background()
	insertWorkflow(t, database, "wf-selection", "Original")
	_, err := database.Exec(`UPDATE workflows SET workspace_id = 'ws-update' WHERE id = 'wf-selection'`)
	require.NoError(t, err)
	provider := &mockWorkflowProvider{}
	provider.addWorkflow("wf-selection", "ws-update", "Original")
	svc.SetWorkflowProvider(provider)
	first := portableWorkflow("First", "Ready")
	second := portableWorkflow("Second", "Done")
	definition := exportOf(first)
	definition.Workflows = append(definition.Workflows, second)
	preview, err := svc.PreviewWorkflowUpdate(ctx, "ws-update", "wf-selection", definition, WorkflowUpdateRequest{})
	require.NoError(t, err)
	require.False(t, preview.CanApply)
	require.Contains(t, preview.Errors, "select a workflow from the portable file")
	preview, err = svc.PreviewWorkflowUpdate(ctx, "ws-update", "wf-selection", definition, WorkflowUpdateRequest{WorkflowIndex: intPointer(0)})
	require.NoError(t, err)
	require.True(t, preview.CanApply)
}
