package orchestrator

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/entryroute"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// OrchestratorEntryProfileSelector resolves a tag-configured workflow step's
// concrete profile for engine/CAS transitions and the read-only move preview.
// It mirrors the task service selector; the same backendapp adapter satisfies
// both.
type OrchestratorEntryProfileSelector interface {
	SelectEntryProfile(ctx context.Context, stepID string, allowedTags []string, fallbackProfileID string, executor *models.Executor, executorProfile *models.ExecutorProfile) (string, error)
}

// entryRouteAttacher is the workflow store's pending-route selector.
type entryRouteAttacher func(ctx context.Context, taskID, sessionID string, step *wfmodels.WorkflowStep) (context.Context, error)

// SetWorkflowEntryProfileSelector wires the selector used to freeze a tagged
// step entry committed by the workflow engine or a guarded CAS transition.
func (s *Service) SetWorkflowEntryProfileSelector(selector OrchestratorEntryProfileSelector) {
	s.workflowEntryProfileSelector = selector
}

// attachEngineEntryRoute resolves a tag-configured target step's concrete
// profile once and attaches it to ctx for the transition write. A task
// fixed-step override wins before quota selection, a task read failure fails
// closed, and the active session's executor is preferred for compatibility.
// A step without allowed tags is a no-op.
func (s *Service) attachEngineEntryRoute(
	ctx context.Context,
	taskID, sessionID string,
	step *wfmodels.WorkflowStep,
) (context.Context, error) {
	if step == nil || len(step.AllowedTags) == 0 || step.SessionTarget != nil {
		return ctx, nil
	}
	var task *models.Task
	if s.repo != nil && taskID != "" {
		loaded, err := s.repo.GetTask(ctx, taskID)
		if err != nil {
			return ctx, fmt.Errorf("load task for tagged entry route: %w", err)
		}
		task = loaded
	}
	startPolicy := string(models.NormalizeWorkflowProfileSessionStartPolicy(string(step.ProfileSessionStartPolicy)))
	attach := func(profileID string) context.Context {
		return entryroute.WithPendingRoute(ctx, entryroute.PendingRoute{
			DestinationStepID: step.ID,
			AgentProfileID:    profileID,
			StartPolicy:       startPolicy,
			SourceSessionID:   sessionID,
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
	executor, executorProfile := s.selectionExecutor(ctx, task, sessionID)
	selected, err := s.workflowEntryProfileSelector.SelectEntryProfile(
		ctx, step.ID, step.AllowedTags, step.AgentProfileID, executor, executorProfile,
	)
	if err != nil {
		return ctx, err
	}
	return attach(selected), nil
}

// selectionExecutor resolves the task's effective executor for candidate
// compatibility, preferring the active session's executor and falling back to
// the task's recorded executor IDs. Missing executor context is permissive.
func (s *Service) selectionExecutor(
	ctx context.Context,
	task *models.Task,
	sessionID string,
) (*models.Executor, *models.ExecutorProfile) {
	if s.repo == nil {
		return nil, nil
	}
	executorID, executorProfileID := "", ""
	if sessionID != "" {
		if session, err := s.repo.GetTaskSession(ctx, sessionID); err == nil && session != nil {
			executorID, executorProfileID = session.ExecutorID, session.ExecutorProfileID
		}
	}
	if executorID == "" && task != nil {
		executorID = models.StringFromAny(task.Metadata[models.MetaKeyExecutorID])
		executorProfileID = models.StringFromAny(task.Metadata[models.MetaKeyExecutorProfileID])
	}
	if executorID == "" {
		return nil, nil
	}
	executor, err := s.repo.GetExecutor(ctx, executorID)
	if err != nil || executor == nil {
		return nil, nil
	}
	var executorProfile *models.ExecutorProfile
	if executorProfileID != "" {
		if reader, ok := s.repo.(executorProfileReader); ok {
			executorProfile, _ = reader.GetExecutorProfile(ctx, executorProfileID)
		}
	}
	return executor, executorProfile
}

// executorProfileReader is the narrow executor-profile lookup the effective
// executor resolution needs; satisfied by the concrete task repository.
type executorProfileReader interface {
	GetExecutorProfile(ctx context.Context, id string) (*models.ExecutorProfile, error)
}
