package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/task/transitionfailure"
)

// TransitionFailureRecord is a task's current unresolved asynchronous
// transition failure with its envelope.
type TransitionFailureRecord struct {
	TaskID    string                    `json:"task_id"`
	AttemptID string                    `json:"attempt_id"`
	Source    string                    `json:"source"`
	CreatedAt time.Time                 `json:"created_at"`
	Failure   transitionfailure.Failure `json:"failure"`
}

// ClassifyTransitionFailure describes a declined or failed move of taskID for
// any adapter. Authorization and not-found causes share one redacted shape.
func (s *Service) ClassifyTransitionFailure(ctx context.Context, taskID string, err error) transitionfailure.Failure {
	input := transitionfailure.Input{NotVisible: IsForbidden(err) || errors.Is(err, repoerrors.ErrTaskNotFound)}
	if errors.Is(err, repoerrors.ErrTaskCompletionGateBlocked) {
		// A failed read leaves Gate nil; the refusal is still reported.
		input.Gate, _ = s.GetTaskCompletionGate(ctx, taskID)
	}
	return transitionfailure.Classify(err, input)
}

// RecordAsyncTransitionFailure persists and publishes the failure of a move or
// advancement that was accepted earlier, so the UI and a later agent turn can
// read it after the originating call returned. It supersedes the task's
// previous unresolved failure. Internal callers only.
func (s *Service) RecordAsyncTransitionFailure(ctx context.Context, taskID, attemptID, source, sessionID string, cause error) {
	repo, ok := s.tasks.(repository.TaskTransitionFailureRepository)
	if !ok || cause == nil {
		return
	}
	task, err := s.tasks.GetTask(ctx, taskID)
	if err != nil {
		s.logger.Warn("transition failure not recorded: task unreadable", zap.String("task_id", taskID), zap.Error(err))
		return
	}
	failure := s.ClassifyTransitionFailure(ctx, taskID, cause)
	raw, err := json.Marshal(failure)
	if err != nil {
		s.logger.Error("transition failure not recorded: envelope unserializable", zap.String("task_id", taskID), zap.Error(err))
		return
	}
	record := models.TaskTransitionFailure{
		TaskID: taskID, AttemptID: attemptID, Source: source, SessionID: sessionID,
		Failure: string(raw), CreatedAt: time.Now().UTC(),
	}
	if err := repo.SetTaskTransitionFailure(ctx, record); err != nil {
		s.logger.Error("failed to persist transition failure", zap.String("task_id", taskID), zap.Error(err))
		return
	}
	s.logger.Warn("asynchronous workflow transition failed",
		zap.String("task_id", taskID), zap.String("attempt_id", attemptID), zap.String("source", source),
		zap.String("code", failure.Code), zap.String("incident_id", failure.IncidentID), zap.Error(cause))
	s.publishTransitionFailure(ctx, task, &TransitionFailureRecord{
		TaskID: taskID, AttemptID: attemptID, Source: source, CreatedAt: record.CreatedAt, Failure: failure,
	})
}

// ResolveTransitionFailure clears the task's unresolved failure once a later
// transition succeeded. Internal callers only.
func (s *Service) ResolveTransitionFailure(ctx context.Context, taskID string) {
	repo, ok := s.tasks.(repository.TaskTransitionFailureRepository)
	if !ok {
		return
	}
	cleared, err := repo.ClearTaskTransitionFailure(ctx, taskID)
	if err != nil {
		s.logger.Warn("failed to clear transition failure", zap.String("task_id", taskID), zap.Error(err))
		return
	}
	if !cleared {
		return
	}
	if task, err := s.tasks.GetTask(ctx, taskID); err == nil {
		s.publishTransitionFailure(ctx, task, nil)
	}
}

// GetTransitionFailure returns the task's current unresolved asynchronous
// transition failure, or nil.
func (s *Service) GetTransitionFailure(ctx context.Context, taskID string) (*TransitionFailureRecord, error) {
	if err := s.authorizeTaskScope(ctx, taskID, authz.ScopeWorkspaceRead); err != nil {
		return nil, err
	}
	repo, ok := s.tasks.(repository.TaskTransitionFailureRepository)
	if !ok {
		return nil, nil
	}
	stored, err := repo.GetTaskTransitionFailure(ctx, taskID)
	if err != nil || stored == nil {
		return nil, err
	}
	record := &TransitionFailureRecord{
		TaskID: stored.TaskID, AttemptID: stored.AttemptID, Source: stored.Source, CreatedAt: stored.CreatedAt,
	}
	if err := json.Unmarshal([]byte(stored.Failure), &record.Failure); err != nil {
		return nil, err
	}
	return record, nil
}

func (s *Service) publishTransitionFailure(ctx context.Context, task *models.Task, record *TransitionFailureRecord) {
	if s.eventBus == nil {
		return
	}
	payload := map[string]interface{}{"task_id": task.ID, "workspace_id": task.WorkspaceID, "record": record}
	event := bus.NewEvent(events.TaskTransitionFailureUpdated, "task-service", payload)
	if err := s.eventBus.Publish(ctx, events.TaskTransitionFailureUpdated, event); err != nil {
		s.logger.Warn("failed to publish transition failure event", zap.String("task_id", task.ID), zap.Error(err))
	}
}
