package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/entryroute"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// ErrNoEligibleEntryProfile is returned when a tag-configured step has no
// eligible candidate and no safe static fallback. The move/launch must fail
// before committing rather than guessing a profile.
var ErrNoEligibleEntryProfile = errors.New("no eligible agent profile for workflow step")

// WorkflowEntryProfileSelector resolves the concrete agent profile to freeze
// for one actual workflow entry. Implementations score eligible tagged
// profiles by subscription quota; the task service calls it only on the write
// path, before the transition commits.
type WorkflowEntryProfileSelector interface {
	// SelectEntryProfile returns the concrete profile ID to freeze for an
	// entry into the step with the given allowed tags, falling back to
	// fallbackProfileID when no tagged candidate is safe to use. It returns
	// ErrNoEligibleEntryProfile when neither exists. stepID is for
	// observability only; executor/executorProfile exclude candidates the
	// task's executor cannot run.
	SelectEntryProfile(ctx context.Context, stepID string, allowedTags []string, fallbackProfileID string, executor *models.Executor, executorProfile *models.ExecutorProfile) (string, error)
}

// SetWorkflowEntryProfileSelector wires the selector used to freeze a tagged
// step entry's concrete profile. Optional; without it tagged steps keep their
// fixed-profile resolution.
func (s *Service) SetWorkflowEntryProfileSelector(selector WorkflowEntryProfileSelector) {
	s.workflowEntryProfileSelector = selector
}

// AttachPendingEntryRouteForStep resolves a tagged step's concrete profile and
// attaches it to ctx for the next step-transition write. Callers outside this
// package (for example the Office workflow switcher) use it to freeze an entry
// committed by a repository path that does not otherwise pass through this
// service. Untagged steps, unknown tasks, and steps with a session target are
// no-ops; a tagged step with no eligible profile and no safe fallback returns
// the selector error so the caller fails before committing.
func (s *Service) AttachPendingEntryRouteForStep(ctx context.Context, taskID, stepID string) (context.Context, error) {
	if taskID == "" || stepID == "" || s.workflowStepGetter == nil {
		return ctx, nil
	}
	task, err := s.tasks.GetTask(ctx, taskID)
	if err != nil {
		return ctx, fmt.Errorf("load task %q for workflow entry route: %w", taskID, err)
	}
	if task == nil {
		return ctx, nil
	}
	step, err := s.workflowStepGetter.GetStep(ctx, stepID)
	if err != nil {
		return ctx, fmt.Errorf("load workflow step %q for entry route: %w", stepID, err)
	}
	if step == nil || len(step.AllowedTags) == 0 || step.SessionTarget != nil {
		return ctx, nil
	}
	return s.attachPendingEntryRoute(ctx, task, step, nil)
}

// attachPendingEntryRoute resolves a tag-configured step's concrete profile
// once and attaches it to ctx for the transition write. A step without allowed
// tags, or an unwired selector, is a no-op. A task fixed-step override wins
// before any quota selection.
func (s *Service) attachPendingEntryRoute(
	ctx context.Context,
	task *models.Task,
	step *wfmodels.WorkflowStep,
	sourceSession *models.TaskSession,
) (context.Context, error) {
	if step == nil || len(step.AllowedTags) == 0 {
		return ctx, nil
	}
	sourceSessionID := ""
	if sourceSession != nil {
		sourceSessionID = sourceSession.ID
	}
	startPolicy := string(models.NormalizeWorkflowProfileSessionStartPolicy(string(step.ProfileSessionStartPolicy)))
	attach := func(profileID string) context.Context {
		return entryroute.WithPendingRoute(ctx, entryroute.PendingRoute{
			DestinationStepID: step.ID,
			AgentProfileID:    profileID,
			StartPolicy:       startPolicy,
			SourceSessionID:   sourceSessionID,
		})
	}
	if task != nil {
		if replacement, ok := task.WorkflowAgentOverrides.ReplacementFor(step.WorkflowID, step.ID); ok && replacement != "" {
			return attach(replacement), nil
		}
	}
	if s.workflowEntryProfileSelector == nil {
		return ctx, fmt.Errorf("%w: cannot freeze tagged step %s", entryroute.ErrSelectorUnavailable, step.ID)
	}
	executor, executorProfile := s.selectionExecutor(ctx, task, sourceSession)
	selected, err := s.workflowEntryProfileSelector.SelectEntryProfile(
		ctx, step.ID, step.AllowedTags, step.AgentProfileID, executor, executorProfile,
	)
	if err != nil {
		return ctx, err
	}
	return attach(selected), nil
}

// selectionExecutor resolves the task's effective executor for candidate
// compatibility. Missing executor context is permissive: the selector then
// skips the compatibility filter rather than failing the entry.
func (s *Service) selectionExecutor(
	ctx context.Context,
	task *models.Task,
	session *models.TaskSession,
) (*models.Executor, *models.ExecutorProfile) {
	if s.executors == nil {
		return nil, nil
	}
	executorID, executorProfileID := "", ""
	if session != nil {
		executorID, executorProfileID = session.ExecutorID, session.ExecutorProfileID
	}
	if executorID == "" && task != nil {
		executorID = models.StringFromAny(task.Metadata[models.MetaKeyExecutorID])
		executorProfileID = models.StringFromAny(task.Metadata[models.MetaKeyExecutorProfileID])
	}
	if executorID == "" {
		return nil, nil
	}
	executor, err := s.executors.GetExecutor(ctx, executorID)
	if err != nil || executor == nil {
		return nil, nil
	}
	var executorProfile *models.ExecutorProfile
	if executorProfileID != "" {
		executorProfile, _ = s.executors.GetExecutorProfile(ctx, executorProfileID)
	}
	return executor, executorProfile
}
