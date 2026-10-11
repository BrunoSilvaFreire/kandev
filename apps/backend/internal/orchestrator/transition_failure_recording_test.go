package orchestrator

import (
	"context"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type transitionFailureSinkSpy struct {
	mu       sync.Mutex
	recorded []string
	sources  []string
	causes   []error
	resolved []string
}

func (s *transitionFailureSinkSpy) PublishTaskUpdated(context.Context, *models.Task, ...string) {}
func (s *transitionFailureSinkSpy) PublishTaskStateChanged(context.Context, *models.Task, v1.TaskState) {
}
func (s *transitionFailureSinkSpy) PublishTaskActivityIfChanged(context.Context, string) {}

func (s *transitionFailureSinkSpy) RecordAsyncTransitionFailure(_ context.Context, taskID, attemptID, source, _ string, cause error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recorded = append(s.recorded, taskID+"/"+attemptID)
	s.sources = append(s.sources, source)
	s.causes = append(s.causes, cause)
}

func (s *transitionFailureSinkSpy) ResolveTransitionFailure(_ context.Context, taskID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resolved = append(s.resolved, taskID)
}

func TestApplyPendingMove_RecordsADeclinedDeferredMoveByMoveID(t *testing.T) {
	sc := buildPendingMoveScenario(t)
	spy := &transitionFailureSinkSpy{}
	sc.svc.SetTaskEventPublisher(spy)
	session, err := sc.repo.GetTaskSession(sc.ctx, sc.reviewSessionID)
	if err != nil {
		t.Fatalf("load review session: %v", err)
	}

	// The target step exists but belongs to a different workflow than the move
	// names, so the accepted move is dropped at turn end.
	sc.svc.applyPendingMove(sc.ctx, "task-1", sc.reviewSessionID, session, &messagequeue.PendingMove{
		MoveID: "move-declined", TaskID: "task-1", WorkflowID: "wf-other", WorkflowStepID: stepInProgressID,
	})

	spy.mu.Lock()
	defer spy.mu.Unlock()
	if len(spy.recorded) != 1 || spy.recorded[0] != "task-1/move-declined" {
		t.Fatalf("recorded = %v, want one failure keyed by the move ID", spy.recorded)
	}
	if spy.sources[0] != models.TransitionFailureSourceDeferredMove || spy.causes[0] == nil {
		t.Fatalf("source/cause = %q/%v", spy.sources[0], spy.causes[0])
	}
	if len(spy.resolved) != 0 {
		t.Fatalf("a declined move must not resolve a failure: %v", spy.resolved)
	}
}

func TestApplyPendingMove_ResolvesAnEarlierFailureOnceAMoveApplies(t *testing.T) {
	sc := buildPendingMoveScenario(t)
	spy := &transitionFailureSinkSpy{}
	sc.svc.SetTaskEventPublisher(spy)
	session, err := sc.repo.GetTaskSession(sc.ctx, sc.reviewSessionID)
	if err != nil {
		t.Fatalf("load review session: %v", err)
	}

	sc.svc.applyPendingMove(sc.ctx, "task-1", sc.reviewSessionID, session, &messagequeue.PendingMove{
		MoveID: "move-ok", TaskID: "task-1", WorkflowID: "wf1", WorkflowStepID: stepInProgressID,
	})

	spy.mu.Lock()
	defer spy.mu.Unlock()
	if len(spy.recorded) != 0 {
		t.Fatalf("an applied move recorded a failure: %v", spy.recorded)
	}
	if len(spy.resolved) != 1 || spy.resolved[0] != "task-1" {
		t.Fatalf("resolved = %v, want task-1", spy.resolved)
	}
}
