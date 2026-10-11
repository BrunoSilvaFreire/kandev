package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/transitionfailure"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func transitionFailureEvents(eventBus *MockEventBus) []*bus.Event {
	var found []*bus.Event
	for _, evt := range eventBus.GetPublishedEvents() {
		if evt.Type == events.TaskTransitionFailureUpdated {
			found = append(found, evt)
		}
	}
	return found
}

// A transition that fails after its move or signal was accepted stays readable
// until a later transition succeeds, and a newer attempt replaces it.
func TestService_AsyncTransitionFailureIsDurableUntilALaterMoveSucceeds(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-123", WorkspaceID: "ws-1", Name: "Workflow"})
	_ = repo.CreateTask(ctx, &models.Task{
		ID: "task-123", WorkspaceID: "ws-1", WorkflowID: "wf-123", WorkflowStepID: "step-todo",
		Title: "Test", State: v1.TaskStateTODO, Priority: "medium",
	})
	eventBus.ClearEvents()

	svc.RecordAsyncTransitionFailure(ctx, "task-123", "move-1", models.TransitionFailureSourceDeferredMove, "s1",
		errors.New("driver: connection reset by peer"))
	svc.RecordAsyncTransitionFailure(ctx, "task-123", "advance-2", models.TransitionFailureSourceAutoAdvance, "s1",
		wfmodels.ErrWIPLimitExceeded)

	record, err := svc.GetTransitionFailure(ctx, "task-123")
	if err != nil || record == nil {
		t.Fatalf("read = %+v, %v", record, err)
	}
	if record.AttemptID != "advance-2" || record.Source != models.TransitionFailureSourceAutoAdvance ||
		record.Failure.Code != transitionfailure.CodeWIPLimit || len(record.Failure.Reasons) != 1 {
		t.Fatalf("the newer attempt must replace the older one: %+v", record)
	}
	published := transitionFailureEvents(eventBus)
	if len(published) != 2 {
		t.Fatalf("published %d failure events, want 2", len(published))
	}
	payload := published[0].Data.(map[string]interface{})
	first, _ := payload["record"].(*TransitionFailureRecord)
	if payload["workspace_id"] != "ws-1" || first == nil || first.Failure.Code != transitionfailure.CodeInternal ||
		first.Failure.IncidentID == "" {
		t.Fatalf("first event payload = %+v", payload)
	}

	if _, err := svc.MoveTask(ctx, "task-123", "wf-123", "step-done", 0); err != nil {
		t.Fatalf("move: %v", err)
	}
	if record, _ := svc.GetTransitionFailure(ctx, "task-123"); record != nil {
		t.Fatalf("a successful move must resolve the failure, got %+v", record)
	}
	published = transitionFailureEvents(eventBus)
	last := published[len(published)-1].Data.(map[string]interface{})
	if cleared, _ := last["record"].(*TransitionFailureRecord); len(published) != 3 || cleared != nil {
		t.Fatalf("expected a third event with a null record, got %d events, last %+v", len(published), last)
	}
}
