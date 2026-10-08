package orchestrator

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
)

type possibleQuestionTestRepo struct {
	repoStore
	mu              sync.Mutex
	tasks           map[string]*models.Task
	sessions        map[string]*models.TaskSession
	messages        map[string][]*models.Message
	pendingActions  map[string]models.TaskPendingAction
	metadataKeysSet map[string]map[string]interface{}
}

func newPossibleQuestionTestRepo() *possibleQuestionTestRepo {
	return &possibleQuestionTestRepo{
		tasks:           make(map[string]*models.Task),
		sessions:        make(map[string]*models.TaskSession),
		messages:        make(map[string][]*models.Message),
		pendingActions:  make(map[string]models.TaskPendingAction),
		metadataKeysSet: make(map[string]map[string]interface{}),
	}
}

func (r *possibleQuestionTestRepo) GetTask(_ context.Context, id string) (*models.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.tasks[id]
	if !ok {
		return nil, errors.New("task not found")
	}
	cp := *t
	return &cp, nil
}

func (r *possibleQuestionTestRepo) GetTaskSession(_ context.Context, id string) (*models.TaskSession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sessions[id]
	if !ok {
		return nil, errors.New("session not found")
	}
	cp := *s
	return &cp, nil
}

func (r *possibleQuestionTestRepo) SetSessionMetadataKey(_ context.Context, sessionID, key string, value interface{}) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.metadataKeysSet[sessionID]; !ok {
		r.metadataKeysSet[sessionID] = make(map[string]interface{})
	}
	r.metadataKeysSet[sessionID][key] = value
	if s, ok := r.sessions[sessionID]; ok {
		if s.Metadata == nil {
			s.Metadata = make(map[string]interface{})
		}
		s.Metadata[key] = value
	}
	return nil
}

func (r *possibleQuestionTestRepo) ListMessages(_ context.Context, sessionID string) ([]*models.Message, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.messages[sessionID], nil
}

func (r *possibleQuestionTestRepo) GetPendingActionsBySessionIDs(_ context.Context, sessionIDs []string) (map[string]models.TaskPendingAction, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	res := make(map[string]models.TaskPendingAction)
	for _, id := range sessionIDs {
		if action, ok := r.pendingActions[id]; ok {
			res[id] = action
		}
	}
	return res, nil
}

func (r *possibleQuestionTestRepo) wasMetadataKeySet(sessionID, key string) (interface{}, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if m, ok := r.metadataKeysSet[sessionID]; ok {
		val, exists := m[key]
		return val, exists
	}
	return nil, false
}

func TestOnSessionStateChangedForPossibleQuestion(t *testing.T) {
	ctx := context.Background()

	t.Run("detects trailing question mark on final paragraph", func(t *testing.T) {
		repo := newPossibleQuestionTestRepo()
		svc := &Service{repo: repo, logger: testLogger()}

		session := &models.TaskSession{
			ID:     "s1",
			TaskID: "t1",
			State:  models.TaskSessionStateRunning,
		}
		repo.sessions["s1"] = session
		repo.tasks["t1"] = &models.Task{ID: "t1"}
		repo.messages["s1"] = []*models.Message{
			{
				ID:         "m1",
				AuthorType: models.MessageAuthorAgent,
				Type:       models.MessageTypeMessage,
				Content:    "I have analyzed the requirements.\n\nShould we proceed with Option A or Option B?",
				CreatedAt:  time.Now(),
			},
		}

		svc.onSessionStateChangedForPossibleQuestion(ctx, "t1", "s1", models.TaskSessionStateRunning, models.TaskSessionStateWaitingForInput, session)

		assert.True(t, models.SessionPossibleQuestion(session.Metadata))
		val, exists := repo.wasMetadataKeySet("s1", models.SessionMetaKeyPossibleQuestion)
		require.True(t, exists)
		valMap, ok := val.(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, true, valMap["active"])
		assert.Equal(t, "s1", valMap["session_id"])
	})

	t.Run("detects decision cues without question mark", func(t *testing.T) {
		repo := newPossibleQuestionTestRepo()
		svc := &Service{repo: repo, logger: testLogger()}

		session := &models.TaskSession{
			ID:     "s1",
			TaskID: "t1",
			State:  models.TaskSessionStateRunning,
		}
		repo.sessions["s1"] = session
		repo.tasks["t1"] = &models.Task{ID: "t1"}
		repo.messages["s1"] = []*models.Message{
			{
				ID:         "m1",
				AuthorType: models.MessageAuthorAgent,
				Type:       models.MessageTypeMessage,
				Content:    "Implementation ready. Next move is yours.",
				CreatedAt:  time.Now(),
			},
		}

		svc.onSessionStateChangedForPossibleQuestion(ctx, "t1", "s1", models.TaskSessionStateRunning, models.TaskSessionStateWaitingForInput, session)

		assert.True(t, models.SessionPossibleQuestion(session.Metadata))
	})

	t.Run("detects multiple choice options", func(t *testing.T) {
		repo := newPossibleQuestionTestRepo()
		svc := &Service{repo: repo, logger: testLogger()}

		session := &models.TaskSession{
			ID:     "s1",
			TaskID: "t1",
			State:  models.TaskSessionStateRunning,
		}
		repo.sessions["s1"] = session
		repo.tasks["t1"] = &models.Task{ID: "t1"}
		repo.messages["s1"] = []*models.Message{
			{
				ID:         "m1",
				AuthorType: models.MessageAuthorAgent,
				Type:       models.MessageTypeMessage,
				Content:    "We have two paths:\nOption 1: Migrations first.\nOption 2: Schema direct.",
				CreatedAt:  time.Now(),
			},
		}

		svc.onSessionStateChangedForPossibleQuestion(ctx, "t1", "s1", models.TaskSessionStateRunning, models.TaskSessionStateWaitingForInput, session)

		assert.True(t, models.SessionPossibleQuestion(session.Metadata))
	})

	t.Run("does not flag non-question prose", func(t *testing.T) {
		repo := newPossibleQuestionTestRepo()
		svc := &Service{repo: repo, logger: testLogger()}

		session := &models.TaskSession{
			ID:     "s1",
			TaskID: "t1",
			State:  models.TaskSessionStateRunning,
		}
		repo.sessions["s1"] = session
		repo.tasks["t1"] = &models.Task{ID: "t1"}
		repo.messages["s1"] = []*models.Message{
			{
				ID:         "m1",
				AuthorType: models.MessageAuthorAgent,
				Type:       models.MessageTypeMessage,
				Content:    "I have completed all unit tests successfully.",
				CreatedAt:  time.Now(),
			},
		}

		svc.onSessionStateChangedForPossibleQuestion(ctx, "t1", "s1", models.TaskSessionStateRunning, models.TaskSessionStateWaitingForInput, session)

		assert.False(t, models.SessionPossibleQuestion(session.Metadata))
	})

	t.Run("bypassed when structured clarification is active", func(t *testing.T) {
		repo := newPossibleQuestionTestRepo()
		svc := &Service{repo: repo, logger: testLogger()}

		session := &models.TaskSession{
			ID:     "s1",
			TaskID: "t1",
			State:  models.TaskSessionStateRunning,
		}
		repo.sessions["s1"] = session
		repo.tasks["t1"] = &models.Task{ID: "t1"}
		repo.pendingActions["s1"] = models.TaskPendingActionClarification
		repo.messages["s1"] = []*models.Message{
			{
				ID:         "m1",
				AuthorType: models.MessageAuthorAgent,
				Type:       models.MessageTypeMessage,
				Content:    "Which option would you prefer?",
				CreatedAt:  time.Now(),
			},
		}

		svc.onSessionStateChangedForPossibleQuestion(ctx, "t1", "s1", models.TaskSessionStateRunning, models.TaskSessionStateWaitingForInput, session)

		assert.False(t, models.SessionPossibleQuestion(session.Metadata))
	})

	t.Run("bypassed when pending step completion exists", func(t *testing.T) {
		repo := newPossibleQuestionTestRepo()
		svc := &Service{repo: repo, logger: testLogger()}

		session := &models.TaskSession{
			ID:     "s1",
			TaskID: "t1",
			State:  models.TaskSessionStateRunning,
			Metadata: map[string]interface{}{
				models.SessionMetaKeyPendingStepCompletion: map[string]interface{}{
					"step_id": "step-1",
					"source":  "agent",
				},
			},
		}
		repo.sessions["s1"] = session
		repo.tasks["t1"] = &models.Task{ID: "t1"}
		repo.messages["s1"] = []*models.Message{
			{
				ID:         "m1",
				AuthorType: models.MessageAuthorAgent,
				Type:       models.MessageTypeMessage,
				Content:    "Which option would you prefer?",
				CreatedAt:  time.Now(),
			},
		}

		svc.onSessionStateChangedForPossibleQuestion(ctx, "t1", "s1", models.TaskSessionStateRunning, models.TaskSessionStateWaitingForInput, session)

		assert.False(t, models.SessionPossibleQuestion(session.Metadata))
	})

	t.Run("cleared nil step-completion key does not suppress detection", func(t *testing.T) {
		repo := newPossibleQuestionTestRepo()
		svc := &Service{repo: repo, logger: testLogger()}

		session := &models.TaskSession{
			ID:     "s1",
			TaskID: "t1",
			State:  models.TaskSessionStateRunning,
			Metadata: map[string]interface{}{
				// A cleared bag entry persists as a nil key; it is not a signal.
				models.SessionMetaKeyPendingStepCompletion: nil,
			},
		}
		repo.sessions["s1"] = session
		repo.tasks["t1"] = &models.Task{ID: "t1"}
		repo.messages["s1"] = []*models.Message{
			{
				ID:         "m1",
				AuthorType: models.MessageAuthorAgent,
				Type:       models.MessageTypeMessage,
				Content:    "Which option would you prefer?",
				CreatedAt:  time.Now(),
			},
		}

		svc.onSessionStateChangedForPossibleQuestion(ctx, "t1", "s1", models.TaskSessionStateRunning, models.TaskSessionStateWaitingForInput, session)

		assert.True(t, models.SessionPossibleQuestion(session.Metadata))
	})

	t.Run("bypassed when task has workflow move pending", func(t *testing.T) {
		repo := newPossibleQuestionTestRepo()
		svc := &Service{repo: repo, logger: testLogger()}

		session := &models.TaskSession{
			ID:     "s1",
			TaskID: "t1",
			State:  models.TaskSessionStateRunning,
		}
		repo.sessions["s1"] = session
		repo.tasks["t1"] = &models.Task{
			ID: "t1",
			Metadata: map[string]interface{}{
				models.MetaKeyWorkflowMovePending: true,
			},
		}
		repo.messages["s1"] = []*models.Message{
			{
				ID:         "m1",
				AuthorType: models.MessageAuthorAgent,
				Type:       models.MessageTypeMessage,
				Content:    "Which option would you prefer?",
				CreatedAt:  time.Now(),
			},
		}

		svc.onSessionStateChangedForPossibleQuestion(ctx, "t1", "s1", models.TaskSessionStateRunning, models.TaskSessionStateWaitingForInput, session)

		assert.False(t, models.SessionPossibleQuestion(session.Metadata))
	})

	t.Run("bypassed when in-turn tool call moved task", func(t *testing.T) {
		repo := newPossibleQuestionTestRepo()
		svc := &Service{repo: repo, logger: testLogger()}

		session := &models.TaskSession{
			ID:     "s1",
			TaskID: "t1",
			State:  models.TaskSessionStateRunning,
		}
		repo.sessions["s1"] = session
		repo.tasks["t1"] = &models.Task{ID: "t1"}
		repo.messages["s1"] = []*models.Message{
			{
				ID:         "m0",
				TurnID:     "turn-1",
				AuthorType: models.MessageAuthorAgent,
				Type:       models.MessageTypeToolCall,
				Metadata:   map[string]interface{}{"tool_name": "move_task_kandev"},
				CreatedAt:  time.Now(),
			},
			{
				ID:         "m1",
				TurnID:     "turn-1",
				AuthorType: models.MessageAuthorAgent,
				Type:       models.MessageTypeMessage,
				Content:    "I have moved the task. Next move is yours.",
				CreatedAt:  time.Now().Add(time.Second),
			},
		}

		svc.onSessionStateChangedForPossibleQuestion(ctx, "t1", "s1", models.TaskSessionStateRunning, models.TaskSessionStateWaitingForInput, session)

		assert.False(t, models.SessionPossibleQuestion(session.Metadata))
	})

	t.Run("bypassed when in-turn tool call completed step", func(t *testing.T) {
		repo := newPossibleQuestionTestRepo()
		svc := &Service{repo: repo, logger: testLogger()}

		session := &models.TaskSession{
			ID:     "s1",
			TaskID: "t1",
			State:  models.TaskSessionStateRunning,
		}
		repo.sessions["s1"] = session
		repo.tasks["t1"] = &models.Task{ID: "t1"}
		repo.messages["s1"] = []*models.Message{
			{
				ID:         "m0",
				TurnID:     "turn-1",
				AuthorType: models.MessageAuthorAgent,
				Type:       models.MessageTypeToolCall,
				Metadata:   map[string]interface{}{"tool_name": "step_complete_kandev"},
				CreatedAt:  time.Now(),
			},
			{
				ID:         "m1",
				TurnID:     "turn-1",
				AuthorType: models.MessageAuthorAgent,
				Type:       models.MessageTypeMessage,
				Content:    "Step complete. Next move is yours.",
				CreatedAt:  time.Now().Add(time.Second),
			},
		}

		svc.onSessionStateChangedForPossibleQuestion(ctx, "t1", "s1", models.TaskSessionStateRunning, models.TaskSessionStateWaitingForInput, session)

		assert.False(t, models.SessionPossibleQuestion(session.Metadata))
	})

	t.Run("clears possible_question when leaving WAITING_FOR_INPUT", func(t *testing.T) {
		repo := newPossibleQuestionTestRepo()
		svc := &Service{repo: repo, logger: testLogger()}

		session := &models.TaskSession{
			ID:     "s1",
			TaskID: "t1",
			State:  models.TaskSessionStateWaitingForInput,
			Metadata: map[string]interface{}{
				models.SessionMetaKeyPossibleQuestion: true,
			},
		}
		repo.sessions["s1"] = session
		repo.tasks["t1"] = &models.Task{ID: "t1"}

		svc.onSessionStateChangedForPossibleQuestion(ctx, "t1", "s1", models.TaskSessionStateWaitingForInput, models.TaskSessionStateRunning, session)

		assert.False(t, models.SessionPossibleQuestion(session.Metadata))
		val, exists := repo.wasMetadataKeySet("s1", models.SessionMetaKeyPossibleQuestion)
		require.True(t, exists)
		assert.Equal(t, false, val)
	})

	t.Run("prose merely mentioning move_task does not bypass question detection", func(t *testing.T) {
		repo := newPossibleQuestionTestRepo()
		svc := &Service{repo: repo, logger: testLogger()}

		session := &models.TaskSession{
			ID:     "s1",
			TaskID: "t1",
			State:  models.TaskSessionStateRunning,
		}
		repo.sessions["s1"] = session
		repo.tasks["t1"] = &models.Task{ID: "t1"}
		repo.messages["s1"] = []*models.Message{
			{
				ID:         "m1",
				TurnID:     "turn-1",
				AuthorType: models.MessageAuthorAgent,
				Type:       models.MessageTypeMessage,
				Content:    "I considered calling move_task_kandev, but which option should we implement first?",
				CreatedAt:  time.Now(),
			},
		}

		svc.onSessionStateChangedForPossibleQuestion(ctx, "t1", "s1", models.TaskSessionStateRunning, models.TaskSessionStateWaitingForInput, session)

		assert.True(t, models.SessionPossibleQuestion(session.Metadata))
	})

	t.Run("failed tool call does not bypass question detection", func(t *testing.T) {
		repo := newPossibleQuestionTestRepo()
		svc := &Service{repo: repo, logger: testLogger()}

		session := &models.TaskSession{
			ID:     "s1",
			TaskID: "t1",
			State:  models.TaskSessionStateRunning,
		}
		repo.sessions["s1"] = session
		repo.tasks["t1"] = &models.Task{ID: "t1"}
		repo.messages["s1"] = []*models.Message{
			{
				ID:         "m0",
				TurnID:     "turn-1",
				AuthorType: models.MessageAuthorAgent,
				Type:       models.MessageTypeToolCall,
				Metadata: map[string]interface{}{
					"tool_name":    "move_task_kandev",
					"tool_call_id": "call-1",
				},
				CreatedAt: time.Now(),
			},
			{
				ID:         "m1",
				TurnID:     "turn-1",
				AuthorType: models.MessageAuthorAgent,
				Type:       models.MessageTypeToolExecute,
				Metadata: map[string]interface{}{
					"tool_call_id": "call-1",
					"is_error":     true,
					"error":        "completion gate blocked",
				},
				CreatedAt: time.Now().Add(100 * time.Millisecond),
			},
			{
				ID:         "m2",
				TurnID:     "turn-1",
				AuthorType: models.MessageAuthorAgent,
				Type:       models.MessageTypeMessage,
				Content:    "The move failed because gates are blocked. How would you like to proceed?",
				CreatedAt:  time.Now().Add(200 * time.Millisecond),
			},
		}

		svc.onSessionStateChangedForPossibleQuestion(ctx, "t1", "s1", models.TaskSessionStateRunning, models.TaskSessionStateWaitingForInput, session)

		assert.True(t, models.SessionPossibleQuestion(session.Metadata))
	})

	t.Run("bypassed when task or session is parked on background work", func(t *testing.T) {
		repo := newPossibleQuestionTestRepo()
		svc := &Service{repo: repo, logger: testLogger()}

		session := &models.TaskSession{
			ID:     "s1",
			TaskID: "t1",
			State:  models.TaskSessionStateRunning,
			Metadata: map[string]interface{}{
				"parked_on_background_work": true,
			},
		}
		repo.sessions["s1"] = session
		repo.tasks["t1"] = &models.Task{ID: "t1"}
		repo.messages["s1"] = []*models.Message{
			{
				ID:         "m1",
				TurnID:     "turn-1",
				AuthorType: models.MessageAuthorAgent,
				Type:       models.MessageTypeMessage,
				Content:    "Which option should we pick?",
				CreatedAt:  time.Now(),
			},
		}

		svc.onSessionStateChangedForPossibleQuestion(ctx, "t1", "s1", models.TaskSessionStateRunning, models.TaskSessionStateWaitingForInput, session)

		assert.False(t, models.SessionPossibleQuestion(session.Metadata))
	})

	t.Run("fails closed when pending actions lookup fails", func(t *testing.T) {
		repo := newPossibleQuestionTestRepo()
		svc := &Service{repo: &failingPendingActionsRepo{repo}, logger: testLogger()}

		session := &models.TaskSession{
			ID:     "s1",
			TaskID: "t1",
			State:  models.TaskSessionStateRunning,
		}
		repo.sessions["s1"] = session
		repo.tasks["t1"] = &models.Task{ID: "t1"}
		repo.messages["s1"] = []*models.Message{
			{
				ID:         "m1",
				TurnID:     "turn-1",
				AuthorType: models.MessageAuthorAgent,
				Type:       models.MessageTypeMessage,
				Content:    "Should we do A or B?",
				CreatedAt:  time.Now(),
			},
		}

		svc.onSessionStateChangedForPossibleQuestion(ctx, "t1", "s1", models.TaskSessionStateRunning, models.TaskSessionStateWaitingForInput, session)

		assert.False(t, models.SessionPossibleQuestion(session.Metadata))
	})

	t.Run("unconditionally clears persisted hint even when in-memory metadata is missing", func(t *testing.T) {
		repo := newPossibleQuestionTestRepo()
		svc := &Service{repo: repo, logger: testLogger()}

		// Repo has the key persisted from a prior turn/session
		repo.metadataKeysSet["s1"] = map[string]interface{}{
			models.SessionMetaKeyPossibleQuestion: true,
		}

		// In-memory session has no metadata populated (e.g. after restart or stale struct)
		session := &models.TaskSession{
			ID:     "s1",
			TaskID: "t1",
			State:  models.TaskSessionStateWaitingForInput,
		}

		svc.onSessionStateChangedForPossibleQuestion(ctx, "t1", "s1", models.TaskSessionStateWaitingForInput, models.TaskSessionStateRunning, session)

		val, exists := repo.wasMetadataKeySet("s1", models.SessionMetaKeyPossibleQuestion)
		require.True(t, exists)
		assert.Equal(t, false, val)
	})
}

type failingPendingActionsRepo struct {
	*possibleQuestionTestRepo
}

func (r *failingPendingActionsRepo) GetPendingActionsBySessionIDs(_ context.Context, _ []string) (map[string]models.TaskPendingAction, error) {
	return nil, errors.New("db connection failure")
}
