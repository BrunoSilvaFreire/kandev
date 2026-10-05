package clarification

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recoveryCall struct {
	taskID, stamp, decision string
}

type recordingRecoveryHandler struct {
	calls []recoveryCall
	err   error
}

func (h *recordingRecoveryHandler) ResolveContinuationRecovery(_ context.Context, taskID, stamp, decision string) error {
	h.calls = append(h.calls, recoveryCall{taskID: taskID, stamp: stamp, decision: decision})
	return h.err
}

func continuationRecoveryDeliveryMessage(pendingID, messageID string) *taskmodels.Message {
	q := ContinuationRecoveryQuestion()
	options := make([]any, len(q.Options))
	for i, opt := range q.Options {
		options[i] = map[string]any{"option_id": opt.ID, "label": opt.Label, "description": opt.Description}
	}
	return &taskmodels.Message{
		ID:            messageID,
		TaskID:        "task-1",
		TaskSessionID: "session-1",
		Metadata: map[string]any{
			metaPendingIDKey:  pendingID,
			metaQuestionIDKey: ContinuationRecoveryQuestionID,
			metaStatusKey:     string(StatusPending),
			"continuation_recovery": &ContinuationRecoveryMeta{
				Stamp: "stamp-1", Case: "cold", Reason: "extraction_failed",
			},
			metaQuestionKey: map[string]any{
				"id": ContinuationRecoveryQuestionID, "title": q.Title, "prompt": q.Prompt, "options": options,
			},
		},
	}
}

func newContinuationRecoveryResolverFixture(
	t *testing.T,
	pendingID string,
) (*Resolver, *resolverDeliveryMessageCreator, *stubEventBus, *recordingRecoveryHandler) {
	t.Helper()
	store := NewStore(time.Minute)
	repo := &stubMessageStore{messages: map[string][]*taskmodels.Message{
		pendingID: {continuationRecoveryDeliveryMessage(pendingID, "message-1")},
	}}
	creator := &resolverDeliveryMessageCreator{repo: repo}
	eventBus := &stubEventBus{}
	resolver := NewResolver(store, repo, creator, &stubAuthorizer{}, eventBus, eventBus, nil, logger.Default())
	handler := &recordingRecoveryHandler{}
	resolver.SetContinuationRecoverySupport(handler)
	return resolver, creator, eventBus, handler
}

func recoveryRetryOutcome() Outcome {
	return Outcome{Answers: []Answer{{
		QuestionID:      ContinuationRecoveryQuestionID,
		SelectedOptions: []string{ContinuationRecoveryDecisionRetry},
	}}}
}

func TestResolverContinuationRecoveryForwardsDecisionAndDoesNotResume(t *testing.T) {
	const pendingID = "pending-recovery-forward"
	resolver, _, eventBus, handler := newContinuationRecoveryResolverFixture(t, pendingID)

	resolution, claimed, err := resolver.ResolveBundle(context.Background(), pendingID, recoveryRetryOutcome())

	require.NoError(t, err)
	require.True(t, claimed)
	require.NotNil(t, resolution)
	require.Len(t, handler.calls, 1, "the handler applies the decision exactly once")
	assert.Equal(t, "task-1", handler.calls[0].taskID)
	assert.Equal(t, "stamp-1", handler.calls[0].stamp)
	assert.Equal(t, ContinuationRecoveryDecisionRetry, handler.calls[0].decision)
	assert.Empty(t, eventBus.resumeRequests, "a recovery bundle must not be delivered to the agent")
}

func TestResolverContinuationRecoveryTreatsDismissalAsContinue(t *testing.T) {
	const pendingID = "pending-recovery-dismiss"
	resolver, _, _, handler := newContinuationRecoveryResolverFixture(t, pendingID)

	_, _, err := resolver.ResolveBundle(context.Background(), pendingID, Outcome{Rejected: true})

	require.NoError(t, err)
	require.Len(t, handler.calls, 1)
	assert.Equal(t, ContinuationRecoveryDecisionContinue, handler.calls[0].decision,
		"a dismissed recovery question continues without handoff instead of stranding the entry")
}

func TestResolverContinuationRecoveryHandlerErrorRestoresClaim(t *testing.T) {
	const pendingID = "pending-recovery-error"
	resolver, creator, _, handler := newContinuationRecoveryResolverFixture(t, pendingID)
	handler.err = errors.New("orchestrator failed")

	_, claimed, err := resolver.ResolveBundle(context.Background(), pendingID, recoveryRetryOutcome())

	require.Error(t, err)
	require.True(t, claimed)
	assert.Equal(t, 1, creator.restoreCalls, "the claim is restored so the user can retry")
}

func TestResolverContinuationRecoveryNilHandlerFailsClosed(t *testing.T) {
	const pendingID = "pending-recovery-nil"
	store := NewStore(time.Minute)
	repo := &stubMessageStore{messages: map[string][]*taskmodels.Message{
		pendingID: {continuationRecoveryDeliveryMessage(pendingID, "message-1")},
	}}
	creator := &resolverDeliveryMessageCreator{repo: repo}
	eventBus := &stubEventBus{}
	resolver := NewResolver(store, repo, creator, &stubAuthorizer{}, eventBus, eventBus, nil, logger.Default())

	_, _, err := resolver.ResolveBundle(context.Background(), pendingID, recoveryRetryOutcome())

	require.Error(t, err)
	assert.Equal(t, 1, creator.restoreCalls, "an unconfigured handler must fail closed and restore the claim")
}
