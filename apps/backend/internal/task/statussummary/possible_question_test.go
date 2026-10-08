package statussummary

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
)

func TestTaskStatusSummaryPossibleQuestionSemanticRoundTrip(t *testing.T) {
	summary := TaskStatusSummary{
		PossibleQuestion: true,
	}
	payload, err := summary.SemanticJSON()
	if err != nil {
		t.Fatalf("semantic JSON: %v", err)
	}
	var decoded TaskStatusSummary
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("decode semantic JSON: %v", err)
	}
	if !decoded.PossibleQuestion {
		t.Fatalf("expected possible_question=true after round-trip, got false")
	}
	if !summary.SemanticEqual(decoded) {
		t.Fatalf("expected semantic equal")
	}

	summaryWithout := TaskStatusSummary{
		PossibleQuestion: false,
	}
	if summary.SemanticEqual(summaryWithout) {
		t.Fatalf("expected summaries with different PossibleQuestion to NOT be semantically equal")
	}
}

func TestTaskStatusSummaryPossibleQuestionTurnIDSemanticRoundTrip(t *testing.T) {
	summary := TaskStatusSummary{PossibleQuestion: true, PossibleQuestionTurnID: "turn-7"}
	payload, err := summary.SemanticJSON()
	if err != nil {
		t.Fatalf("semantic JSON: %v", err)
	}
	var decoded TaskStatusSummary
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("decode semantic JSON: %v", err)
	}
	if decoded.PossibleQuestionTurnID != "turn-7" {
		t.Fatalf("expected turn id to survive round-trip, got %q", decoded.PossibleQuestionTurnID)
	}
	sameTurn := TaskStatusSummary{PossibleQuestion: true, PossibleQuestionTurnID: "turn-7"}
	if !summary.SemanticEqual(sameTurn) {
		t.Fatalf("expected summaries with the same turn id to be semantically equal")
	}
	otherTurn := TaskStatusSummary{PossibleQuestion: true, PossibleQuestionTurnID: "turn-8"}
	if summary.SemanticEqual(otherTurn) {
		t.Fatalf("expected summaries with different turn ids to NOT be semantically equal")
	}
}

func TestProjectorPossibleQuestionProjectsTurnIdentity(t *testing.T) {
	_, store, eventBus, _, _ := newProjectorTest(t)
	const taskID = "task-pq-turn"
	const sessionID = "session-pq-turn"

	publishProjectorEvent(t, eventBus, events.TaskSessionStateChanged, events.TaskSessionStateChanged, map[string]interface{}{
		"task_id":            taskID,
		"session_id":         sessionID,
		"primary_session_id": sessionID,
		"is_primary":         true,
		"new_state":          sessionStateWaitingForInput,
		"session_metadata": map[string]interface{}{
			models.SessionMetaKeyPossibleQuestion: map[string]interface{}{
				"active":  true,
				"turn_id": "turn-42",
			},
		},
	})

	got := store.summary(taskID)
	if got == nil || !got.PossibleQuestion {
		t.Fatalf("expected possible_question=true, got %+v", got)
	}
	if got.PossibleQuestionTurnID != "turn-42" {
		t.Fatalf("expected turn id to project, got %q", got.PossibleQuestionTurnID)
	}

	// A later user message clears both the flag and the turn identity.
	publishProjectorEvent(t, eventBus, events.MessageAdded, events.MessageAdded, map[string]interface{}{
		"task_id":     taskID,
		"session_id":  sessionID,
		"author_type": messageTypeUser,
		"type":        "message",
		"content":     "Option A please",
	})
	got = store.summary(taskID)
	if got != nil && (got.PossibleQuestion || got.PossibleQuestionTurnID != "") {
		t.Fatalf("expected possible question and turn id to clear, got %+v", got)
	}
}

func TestProjectorPossibleQuestionLifecycle(t *testing.T) {
	_, store, eventBus, _, _ := newProjectorTest(t)
	const taskID = "task-pq-lifecycle"
	const sessionID = "session-pq-1"

	// 1. Session settles to WAITING_FOR_INPUT with possible_question=true in metadata
	publishProjectorEvent(t, eventBus, events.TaskSessionStateChanged, events.TaskSessionStateChanged, map[string]interface{}{
		"task_id":            taskID,
		"session_id":         sessionID,
		"primary_session_id": sessionID,
		"is_primary":         true,
		"new_state":          sessionStateWaitingForInput,
		"session_metadata": map[string]interface{}{
			models.SessionMetaKeyPossibleQuestion: true,
		},
	})

	got := store.summary(taskID)
	if got == nil {
		t.Fatalf("expected summary to exist")
	}
	if !got.PossibleQuestion {
		t.Fatalf("expected possible_question=true on status summary, got false")
	}

	// 2. User sends a message -> possible_question cleared
	publishProjectorEvent(t, eventBus, events.MessageAdded, events.MessageAdded, map[string]interface{}{
		"task_id":     taskID,
		"session_id":  sessionID,
		"author_type": messageTypeUser,
		"type":        "message",
		"content":     "I want option A",
	})

	got = store.summary(taskID)
	if got != nil && got.PossibleQuestion {
		t.Fatalf("expected possible_question=false after user message, got true")
	}

	// 3. Resurface possible_question again
	publishProjectorEvent(t, eventBus, events.TaskSessionStateChanged, events.TaskSessionStateChanged, map[string]interface{}{
		"task_id":            taskID,
		"session_id":         sessionID,
		"primary_session_id": sessionID,
		"is_primary":         true,
		"new_state":          sessionStateWaitingForInput,
		"session_metadata": map[string]interface{}{
			models.SessionMetaKeyPossibleQuestion: true,
		},
	})

	got = store.summary(taskID)
	if got == nil || !got.PossibleQuestion {
		t.Fatalf("expected possible_question=true again, got %+v", got)
	}

	// 4. TurnStarted -> clears possible_question
	publishProjectorEvent(t, eventBus, events.TurnStarted, events.TurnStarted, map[string]interface{}{
		"task_id":    taskID,
		"session_id": sessionID,
	})

	got = store.summary(taskID)
	if got != nil && got.PossibleQuestion {
		t.Fatalf("expected possible_question=false after TurnStarted, got true")
	}

	// 5. Resurface possible_question again
	publishProjectorEvent(t, eventBus, events.TaskSessionStateChanged, events.TaskSessionStateChanged, map[string]interface{}{
		"task_id":            taskID,
		"session_id":         sessionID,
		"primary_session_id": sessionID,
		"is_primary":         true,
		"new_state":          sessionStateWaitingForInput,
		"session_metadata": map[string]interface{}{
			models.SessionMetaKeyPossibleQuestion: true,
		},
	})

	// 6. TaskStateChanged (workflow move) -> clears possible_question
	publishProjectorEvent(t, eventBus, events.TaskStateChanged, events.TaskStateChanged, map[string]interface{}{
		"task_id":      taskID,
		"workspace_id": "workspace-1",
	})

	got = store.summary(taskID)
	if got != nil && got.PossibleQuestion {
		t.Fatalf("expected possible_question=false after TaskStateChanged, got true")
	}
}

func TestProjectorPossibleQuestionSurvivesReviewMoveWhileSessionWaits(t *testing.T) {
	_, store, eventBus, _, _ := newProjectorTest(t)
	const taskID = "task-pq-review"
	const sessionID = "session-pq-review"

	publishProjectorEvent(t, eventBus, events.TaskSessionStateChanged, events.TaskSessionStateChanged, map[string]interface{}{
		"task_id":            taskID,
		"session_id":         sessionID,
		"primary_session_id": sessionID,
		"is_primary":         true,
		"new_state":          sessionStateWaitingForInput,
		"session_metadata": map[string]interface{}{
			models.SessionMetaKeyPossibleQuestion: map[string]interface{}{
				"active":  true,
				"turn_id": "turn-1",
			},
		},
	})

	// The automatic turn-complete write to REVIEW keeps the session waiting, so
	// the agent's question is still outstanding and the hint must survive.
	publishProjectorEvent(t, eventBus, events.TaskStateChanged, events.TaskStateChanged, map[string]interface{}{
		"task_id":               taskID,
		"workspace_id":          "workspace-1",
		"primary_session_id":    sessionID,
		"primary_session_state": sessionStateWaitingForInput,
	})

	got := store.summary(taskID)
	if got == nil || !got.PossibleQuestion {
		t.Fatalf("expected possible_question to survive a REVIEW move while waiting, got %+v", got)
	}

	// A task state change that reports the session no longer waiting clears it.
	publishProjectorEvent(t, eventBus, events.TaskStateChanged, events.TaskStateChanged, map[string]interface{}{
		"task_id":               taskID,
		"workspace_id":          "workspace-1",
		"primary_session_id":    sessionID,
		"primary_session_state": sessionStateRunning,
	})
	got = store.summary(taskID)
	if got != nil && got.PossibleQuestion {
		t.Fatalf("expected possible_question to clear when the session stops waiting, got %+v", got)
	}
}

func TestProjectorPossibleQuestionSupersededByPendingAction(t *testing.T) {
	// If a real clarification or permission pending action is present,
	// PossibleQuestion is superseded and must project false.
	state := &projectionState{
		possibleQuestion: true,
		sessions: map[string]sessionObservation{
			"s1": {id: "s1", state: sessionStateWaitingForInput, isPrimary: true},
		},
		pending: map[string]string{
			"s1": pendingClarification,
		},
		pendingObserved: true,
	}

	summary := deriveSummary(state)
	if summary.PendingAction != pendingClarification {
		t.Fatalf("expected pending_action=%q, got %q", pendingClarification, summary.PendingAction)
	}
	if summary.PossibleQuestion {
		t.Fatalf("expected possible_question=false when pending_action is present, got true")
	}
}
