package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/assert"
)

type mockQuotaResolver struct {
	calls map[string]int
}

func (m *mockQuotaResolver) ProfileQuotaExhausted(_ context.Context, profileID string) bool {
	if m.calls == nil {
		m.calls = make(map[string]int)
	}
	m.calls[profileID]++
	return profileID == "exhausted-profile"
}

func TestProfileAvailabilityMemo_CallsOncePerProfile(t *testing.T) {
	mock := &mockQuotaResolver{}
	memo := newProfileAvailabilityMemoFromChecker(context.Background(), mock)

	// Empty profile ID
	assert.False(t, memo(""))
	assert.Equal(t, 0, len(mock.calls))

	// First call for profile-1
	assert.False(t, memo("profile-1"))
	assert.Equal(t, 1, mock.calls["profile-1"])

	// Second call for profile-1 should be cached
	assert.False(t, memo("profile-1"))
	assert.Equal(t, 1, mock.calls["profile-1"])

	// Call for exhausted-profile
	assert.True(t, memo("exhausted-profile"))
	assert.Equal(t, 1, mock.calls["exhausted-profile"])

	// Second call for exhausted-profile should be cached
	assert.True(t, memo("exhausted-profile"))
	assert.Equal(t, 1, mock.calls["exhausted-profile"])
}

func TestSessionTargetProfileID(t *testing.T) {
	assert.Equal(t, "", sessionTargetProfileID(nil))
	assert.Equal(t, "", sessionTargetProfileID(&models.TaskSession{}))
	assert.Equal(t, "agent-p", sessionTargetProfileID(&models.TaskSession{AgentProfileID: "agent-p"}))
	assert.Equal(t, "exec-p", sessionTargetProfileID(&models.TaskSession{
		AgentProfileID:     "agent-p",
		ExecutionProfileID: "exec-p",
	}))
}
