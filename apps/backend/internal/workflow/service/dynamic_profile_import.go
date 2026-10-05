package service

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/workflow/models"
)

// DynamicProfileProvisioner resolves the Dynamic Profile bound to an incoming
// portable step. migratedFrom is the scoped provenance marker for a generated
// profile (empty for a user-authored one); displayName is the user-visible
// profile name. Implementations key on migratedFrom or descriptor content so
// repeated imports and syncs are idempotent.
type DynamicProfileProvisioner interface {
	EnsureGeneratedDynamicProfile(
		ctx context.Context,
		migratedFrom, displayName string,
		descriptor models.DynamicAgentProfilePortable,
	) (string, error)
}

// SetDynamicProfileProvisioner wires the provisioner used to materialize
// generated Dynamic Profiles at legacy import/sync boundaries. When unset,
// legacy documents import exactly as before and dynamic steps stay in the
// interactive selection flow.
func (s *Service) SetDynamicProfileProvisioner(provisioner DynamicProfileProvisioner) {
	s.dynamicProvisioner = provisioner
}

// convertLegacyTaggedExport rewrites legacy tagged steps in an incoming export
// into generated Dynamic Profile descriptors and provisions every dynamic step
// in a workflow the import will not skip. It returns a copy of the export plus
// bindings keyed by "<workflow-index>:<position>". With no provisioner, the
// export is returned unchanged. skipNames is nil for sync, which intentionally
// reuses and updates existing workflows.
func (s *Service) convertLegacyTaggedExport(
	ctx context.Context,
	workspaceID string,
	export *models.WorkflowExport,
	skipNames map[string]bool,
) (*models.WorkflowExport, map[string]string, error) {
	bindings := make(map[string]string)
	if export == nil || s.dynamicProvisioner == nil {
		return export, bindings, nil
	}
	snapshot, err := s.concreteProfileSnapshots(ctx)
	if err != nil {
		return nil, nil, err
	}
	converted, err := models.ConvertTaggedWorkflows(export, snapshot, workspaceID, skipNames)
	if err != nil {
		return nil, nil, err
	}
	for workflowIndex, workflow := range converted.Workflows {
		if skipNames[workflow.Name] {
			continue
		}
		for _, step := range workflow.Steps {
			if step.AgentProfile == nil || step.AgentProfile.Dynamic == nil {
				continue
			}
			displayName := workflow.Name + " / " + step.Name
			profileID, provisionErr := s.dynamicProvisioner.EnsureGeneratedDynamicProfile(
				ctx, step.AgentProfile.Dynamic.MigratedFrom, displayName, *step.AgentProfile.Dynamic,
			)
			if provisionErr != nil {
				return nil, nil, fmt.Errorf("provision dynamic profile for %s: %w", step.AgentProfile.Dynamic.MigratedFrom, provisionErr)
			}
			bindings[fmt.Sprintf("%d:%d", workflowIndex, step.Position)] = profileID
		}
	}
	return converted, bindings, nil
}

func (s *Service) concreteProfileSnapshots(ctx context.Context) ([]models.ConcreteProfileSnapshot, error) {
	if s.importProfileCatalog == nil {
		return nil, nil
	}
	candidates, err := s.importProfileCatalog.ListEligibleProfiles(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: list import profile candidates: %w", ErrImportProfileCatalogUnavailable, err)
	}
	snapshot := make([]models.ConcreteProfileSnapshot, 0, len(candidates))
	for _, candidate := range candidates {
		snapshot = append(snapshot, models.ConcreteProfileSnapshot{
			ID:        candidate.ID,
			Tags:      candidate.Tags,
			AgentName: candidate.AgentName,
			Model:     candidate.Model,
			Mode:      candidate.Mode,
		})
	}
	return snapshot, nil
}

// dynamicStepNeedsProvisioning reports whether a dynamic portable step is
// resolved by the provisioner rather than the interactive selection flow.
func (s *Service) dynamicStepNeedsProvisioning(step models.StepPortable) bool {
	return s.dynamicProvisioner != nil && isDynamicPortableStep(step)
}

// dynamicBindingsForWorkflow extracts one workflow's position→profile bindings.
func dynamicBindingsForWorkflow(bindings map[string]string, workflowIndex int) map[int]string {
	result := make(map[int]string)
	for key, profileID := range bindings {
		index, position, err := parseImportStepKey(key)
		if err != nil || index != workflowIndex {
			continue
		}
		result[position] = profileID
	}
	return result
}

// mergeDynamicBindings folds provisioned generated-profile bindings into the
// direct binding map. Existing direct bindings win, so a provisioned profile
// never overrides a caller-selected one.
func mergeDynamicBindings(direct map[int]map[int]string, dynamic map[string]string) {
	for key, profileID := range dynamic {
		index, position, err := parseImportStepKey(key)
		if err != nil {
			continue
		}
		if direct[index] == nil {
			direct[index] = make(map[int]string)
		}
		if direct[index][position] == "" {
			direct[index][position] = profileID
		}
	}
}

func parseImportStepKey(key string) (int, int, error) {
	var index, position int
	if _, err := fmt.Sscanf(key, "%d:%d", &index, &position); err != nil {
		return 0, 0, err
	}
	return index, position, nil
}

func isDynamicPortableStep(step models.StepPortable) bool {
	return step.AgentProfile != nil && step.AgentProfile.Dynamic != nil
}

// applyDynamicStepBinding binds a generated dynamic step to its provisioned
// profile. Concrete steps and missing bindings are left untouched.
func applyDynamicStepBinding(step *models.WorkflowStep, sp models.StepPortable, bindings map[int]string) {
	if step == nil || !isDynamicPortableStep(sp) {
		return
	}
	if profileID := bindings[sp.Position]; profileID != "" {
		step.AgentProfileID = profileID
	}
}
