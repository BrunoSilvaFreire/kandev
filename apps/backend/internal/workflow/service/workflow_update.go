package service

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"

	"github.com/google/uuid"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/models"
)

type WorkflowUpdateRequest struct {
	WorkflowIndex  *int              `json:"workflow_index,omitempty"`
	RenameMappings map[string]string `json:"rename_mappings,omitempty"`
}

type WorkflowUpdateChange struct {
	Kind   string      `json:"kind"`
	Path   string      `json:"path"`
	Before interface{} `json:"before,omitempty"`
	After  interface{} `json:"after,omitempty"`
}

type WorkflowUpdatePreview struct {
	WorkflowID string                 `json:"workflow_id"`
	Version    string                 `json:"version"`
	Changes    []WorkflowUpdateChange `json:"changes"`
	Errors     []string               `json:"errors"`
	CanApply   bool                   `json:"can_apply"`
}

type workflowUpdatePlan struct {
	workflow *taskmodels.Workflow
	steps    []*models.WorkflowStep
	retired  []string
	changes  []WorkflowUpdateChange
}

func (s *Service) PreviewWorkflowUpdate(ctx context.Context, workspaceID, workflowID string, export *models.WorkflowExport, request WorkflowUpdateRequest) (*WorkflowUpdatePreview, error) {
	if err := s.AuthorizeWorkspace(ctx, workspaceID); err != nil {
		return nil, err
	}
	workflow, err := s.workflowProvider.GetWorkflow(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	if workflow.WorkspaceID != workspaceID {
		return nil, ErrNotVisible
	}
	if err := s.AuthorizeWorkflow(ctx, workflowID); err != nil {
		return nil, err
	}
	if err := s.EnsureWorkflowMutable(ctx, workflowID); err != nil {
		return nil, err
	}
	plan, err := s.prepareWorkflowUpdate(ctx, workflow, export, request)
	if err != nil {
		return &WorkflowUpdatePreview{WorkflowID: workflowID, Errors: []string{err.Error()}, Changes: []WorkflowUpdateChange{}, CanApply: false}, nil
	}
	version, err := s.repo.WorkflowDefinitionVersion(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	for _, stepID := range plan.retired {
		if err := s.repo.CheckStepRetirement(ctx, stepID); err != nil {
			plan.changes = append(plan.changes, WorkflowUpdateChange{Kind: "blocked", Path: "steps." + stepID, Before: err.Error()})
		}
	}
	return &WorkflowUpdatePreview{WorkflowID: workflowID, Version: version, Changes: plan.changes, Errors: []string{}, CanApply: !hasBlockedChange(plan.changes)}, nil
}

func (s *Service) ApplyWorkflowUpdate(ctx context.Context, workspaceID, workflowID, version string, export *models.WorkflowExport, request WorkflowUpdateRequest) error {
	if err := s.AuthorizeWorkspace(ctx, workspaceID); err != nil {
		return err
	}
	workflow, err := s.workflowProvider.GetWorkflow(ctx, workflowID)
	if err != nil {
		return err
	}
	if workflow.WorkspaceID != workspaceID {
		return ErrNotVisible
	}
	if err := s.AuthorizeWorkflow(ctx, workflowID); err != nil {
		return err
	}
	if err := s.EnsureWorkflowMutable(ctx, workflowID); err != nil {
		return err
	}
	plan, err := s.prepareWorkflowUpdate(ctx, workflow, export, request)
	if err != nil {
		return err
	}
	_, err = s.repo.ApplyWorkflowDefinition(ctx, workflowID, version, plan.workflow, plan.steps, plan.retired)
	return err
}

func (s *Service) prepareWorkflowUpdate(ctx context.Context, workflow *taskmodels.Workflow, export *models.WorkflowExport, request WorkflowUpdateRequest) (*workflowUpdatePlan, error) {
	portable, err := selectPortableWorkflow(export, request)
	if err != nil {
		return nil, err
	}
	existing, err := s.repo.ListStepsByWorkflow(ctx, workflow.ID)
	if err != nil {
		return nil, err
	}
	matchedByPosition, retired, err := s.matchPortableSteps(ctx, workflow.ID, existing, portable.Steps, request.RenameMappings)
	if err != nil {
		return nil, err
	}
	plan := &workflowUpdatePlan{workflow: cloneTaskWorkflow(workflow)}
	plan.workflow.Name = portable.Name
	plan.workflow.Description = portable.Description
	plan.workflow.Prompt = portable.Prompt
	plan.workflow.AgentProfileID = ""
	if portable.AgentProfile != nil && s.matchProfile != nil {
		plan.workflow.AgentProfileID = s.matchProfile(portable.AgentProfile.AgentName, portable.AgentProfile.Model, portable.AgentProfile.Mode, workflow.AgentProfileID)
	}
	if plan.steps, err = s.buildUpdateStepCandidates(workflow.ID, portable.Steps, matchedByPosition); err != nil {
		return nil, err
	}
	if err := validateWorkflowSessionTargets(plan.steps); err != nil {
		return nil, fmt.Errorf("invalid workflow session targets: %w", err)
	}
	for _, step := range retired {
		plan.retired = append(plan.retired, step.ID)
	}
	sort.Strings(plan.retired)
	plan.changes = diffWorkflowDefinition(workflow, existing, plan.workflow, plan.steps, retired)
	return plan, nil
}

func selectPortableWorkflow(export *models.WorkflowExport, request WorkflowUpdateRequest) (*models.WorkflowPortable, error) {
	if export == nil {
		return nil, fmt.Errorf("workflow definition is required")
	}
	if err := export.NormalizeCompletionPolicy(); err != nil {
		return nil, fmt.Errorf("invalid export data: %w", err)
	}
	if err := export.Validate(); err != nil {
		return nil, fmt.Errorf("invalid export data: %w", err)
	}
	if request.WorkflowIndex == nil {
		if len(export.Workflows) == 1 {
			return &export.Workflows[0], nil
		}
		return nil, fmt.Errorf("select a workflow from the portable file")
	}
	if *request.WorkflowIndex < 0 || *request.WorkflowIndex >= len(export.Workflows) {
		return nil, fmt.Errorf("select a workflow from the portable file")
	}
	return &export.Workflows[*request.WorkflowIndex], nil
}

// existingStepIndex resolves an imported step to the step it continues.
type existingStepIndex struct {
	steps  []*models.WorkflowStep
	byKey  map[string]*models.WorkflowStep
	byName map[string]*models.WorkflowStep
}

func indexExistingSteps(existing []*models.WorkflowStep) (*existingStepIndex, error) {
	index := &existingStepIndex{
		steps:  existing,
		byKey:  make(map[string]*models.WorkflowStep, len(existing)),
		byName: make(map[string]*models.WorkflowStep, len(existing)),
	}
	for _, step := range existing {
		if step.PortableKey != "" {
			index.byKey[step.PortableKey] = step
		}
		if index.byName[step.Name] != nil {
			return nil, fmt.Errorf("workflow has duplicate step names; repair them before importing an update")
		}
		index.byName[step.Name] = step
	}
	return index, nil
}

// renamedFrom returns the existing step an explicit old-ID-to-new-name mapping
// points at for the imported name, or nil.
func (i *existingStepIndex) renamedFrom(name string, renameMappings map[string]string) *models.WorkflowStep {
	var prior *models.WorkflowStep
	for oldID, newName := range renameMappings {
		if newName != name {
			continue
		}
		for _, candidate := range i.steps {
			if candidate.ID == oldID {
				prior = candidate
			}
		}
	}
	return prior
}

// priorStep finds the existing step an imported step continues: by stable key
// when the file carries one, else by unique name, else by an explicit rename
// mapping. A key that names a retired step is refused.
func (s *Service) priorStep(ctx context.Context, workflowID string, step models.StepPortable, index *existingStepIndex, renameMappings map[string]string) (*models.WorkflowStep, error) {
	var prior *models.WorkflowStep
	if step.Key == "" {
		prior = index.byName[step.Name]
	} else if prior = index.byKey[step.Key]; prior == nil {
		retired, retiredErr := s.repo.GetStepByPortableKey(ctx, workflowID, step.Key)
		if retiredErr == nil && retired.RetiredAt != nil {
			return nil, fmt.Errorf("portable step key %q belongs to a retired historical step and cannot be reused", step.Key)
		}
	}
	if prior == nil {
		prior = index.renamedFrom(step.Name, renameMappings)
	}
	return prior, nil
}

// matchPortableSteps pairs imported steps with existing ones by position and
// returns the existing steps nothing matched, which the update retires.
func (s *Service) matchPortableSteps(
	ctx context.Context,
	workflowID string,
	existing []*models.WorkflowStep,
	portable []models.StepPortable,
	renameMappings map[string]string,
) (map[int]*models.WorkflowStep, []*models.WorkflowStep, error) {
	index, err := indexExistingSteps(existing)
	if err != nil {
		return nil, nil, err
	}
	matchedByPosition := make(map[int]*models.WorkflowStep, len(portable))
	matchedIDs := make(map[string]bool, len(portable))
	for _, step := range portable {
		prior, err := s.priorStep(ctx, workflowID, step, index, renameMappings)
		if err != nil {
			return nil, nil, err
		}
		if prior == nil {
			continue
		}
		if matchedIDs[prior.ID] {
			return nil, nil, fmt.Errorf("multiple imported steps match existing step %q", prior.Name)
		}
		matchedByPosition[step.Position] = prior
		matchedIDs[prior.ID] = true
	}
	var retired []*models.WorkflowStep
	for _, step := range existing {
		if !matchedIDs[step.ID] {
			retired = append(retired, step)
		}
	}
	for _, step := range portable {
		if matchedByPosition[step.Position] == nil && step.Key == "" && len(retired) > 0 {
			return nil, nil, fmt.Errorf("step %q could be a rename; provide a stable key or old step ID to new name mapping", step.Name)
		}
	}
	return matchedByPosition, retired, nil
}

// buildUpdateStepCandidates turns the imported steps into step rows, keeping
// the identity and history fields of the steps they continue.
func (s *Service) buildUpdateStepCandidates(workflowID string, portable []models.StepPortable, matchedByPosition map[int]*models.WorkflowStep) ([]*models.WorkflowStep, error) {
	posToID := make(map[int]string, len(portable))
	for _, step := range portable {
		if prior := matchedByPosition[step.Position]; prior != nil {
			posToID[step.Position] = prior.ID
		} else {
			posToID[step.Position] = uuid.NewString()
		}
	}
	candidates := make([]*models.WorkflowStep, 0, len(portable))
	for _, step := range portable {
		prior := matchedByPosition[step.Position]
		candidate := s.stepFromPortableWithMatcher(workflowID, step, posToID, s.matchProfile, profileID(prior))
		if step.AgentProfile != nil && candidate.AgentProfileID == "" {
			return nil, fmt.Errorf("step %q requests an agent profile that is not available in this workspace; resolve the profile before updating", step.Name)
		}
		if prior != nil {
			candidate.PortableKey = prior.PortableKey
			if step.Key != "" {
				candidate.PortableKey = step.Key
			}
			candidate.CreatedAt = prior.CreatedAt
			candidate.UpdatedAt = prior.UpdatedAt
			candidate.StageType = prior.StageType
		} else if candidate.PortableKey == "" {
			candidate.PortableKey = uuid.NewString()
		}
		if err := models.ValidateWorkflowStep(candidate); err != nil {
			return nil, fmt.Errorf("invalid step %q: %w", candidate.Name, err)
		}
		candidates = append(candidates, candidate)
	}
	return candidates, nil
}

func profileID(step *models.WorkflowStep) string {
	if step == nil {
		return ""
	}
	return step.AgentProfileID
}

func cloneTaskWorkflow(workflow *taskmodels.Workflow) *taskmodels.Workflow {
	clone := *workflow
	return &clone
}

func hasBlockedChange(changes []WorkflowUpdateChange) bool {
	for _, change := range changes {
		if change.Kind == "blocked" {
			return true
		}
	}
	return false
}

func diffWorkflowDefinition(oldWorkflow *taskmodels.Workflow, oldSteps []*models.WorkflowStep, newWorkflow *taskmodels.Workflow, newSteps, removed []*models.WorkflowStep) []WorkflowUpdateChange {
	changes := make([]WorkflowUpdateChange, 0)
	for path, beforeAfter := range map[string][2]string{
		"name": {oldWorkflow.Name, newWorkflow.Name}, "description": {oldWorkflow.Description, newWorkflow.Description},
		"prompt": {oldWorkflow.Prompt, newWorkflow.Prompt}, "agent_profile_id": {oldWorkflow.AgentProfileID, newWorkflow.AgentProfileID},
	} {
		if beforeAfter[0] != beforeAfter[1] {
			changes = append(changes, WorkflowUpdateChange{Kind: "modified", Path: "workflow." + path, Before: beforeAfter[0], After: beforeAfter[1]})
		}
	}
	oldByID := make(map[string]*models.WorkflowStep, len(oldSteps))
	for _, step := range oldSteps {
		oldByID[step.ID] = step
	}
	for _, step := range newSteps {
		prior := oldByID[step.ID]
		if prior == nil {
			data, _ := json.Marshal(step)
			changes = append(changes, WorkflowUpdateChange{Kind: "added", Path: "steps." + step.PortableKey, After: json.RawMessage(data)})
			continue
		}
		before, _ := json.Marshal(prior)
		after, _ := json.Marshal(step)
		if !reflect.DeepEqual(before, after) {
			changes = append(changes, WorkflowUpdateChange{Kind: "modified", Path: "steps." + step.PortableKey, Before: json.RawMessage(before), After: json.RawMessage(after)})
		}
	}
	for _, step := range removed {
		data, _ := json.Marshal(step)
		changes = append(changes, WorkflowUpdateChange{Kind: "removed", Path: "steps." + step.PortableKey, Before: json.RawMessage(data)})
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
	return changes
}
