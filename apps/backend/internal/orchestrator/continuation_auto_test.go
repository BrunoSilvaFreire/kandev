package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionCacheCold(t *testing.T) {
	ctx := context.Background()
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	svc.sessionUsageTotals = func(context.Context, string) (*models.TaskUsageTotals, error) { return nil, errors.New("boom") }
	assert.False(t, svc.sessionCacheCold(ctx, "s1"), "a provider error is unknown, not cold")

	svc.sessionUsageTotals = func(context.Context, string) (*models.TaskUsageTotals, error) {
		return &models.TaskUsageTotals{}, nil
	}
	assert.False(t, svc.sessionCacheCold(ctx, "s1"), "no usage events is unknown, not cold")

	recent := time.Now().Add(-10 * time.Minute)
	svc.sessionUsageTotals = func(context.Context, string) (*models.TaskUsageTotals, error) {
		return &models.TaskUsageTotals{EventCount: 1, LastEventAt: &recent}, nil
	}
	assert.False(t, svc.sessionCacheCold(ctx, "s1"), "a recent usage event is warm")

	old := time.Now().Add(-2 * time.Hour)
	svc.sessionUsageTotals = func(context.Context, string) (*models.TaskUsageTotals, error) {
		return &models.TaskUsageTotals{EventCount: 1, LastEventAt: &old}, nil
	}
	assert.True(t, svc.sessionCacheCold(ctx, "s1"), "usage older than the TTL is cold")
}

func TestSessionCacheCold_NilProviderIsUnknown(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	assert.False(t, svc.sessionCacheCold(context.Background(), "s1"))
}

func TestShouldAutoResumeWithHandoff_Gates(t *testing.T) {
	ctx := context.Background()
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	svc.config.ColdCacheAutoHandoff = true
	svc.sessionlessRunner = func(context.Context, string, string) (string, error) { return "handoff", nil }
	old := time.Now().Add(-2 * time.Hour)
	svc.sessionUsageTotals = func(context.Context, string) (*models.TaskUsageTotals, error) {
		return &models.TaskUsageTotals{EventCount: 1, LastEventAt: &old}, nil
	}
	waiting := &models.TaskSession{ID: "s1", State: models.TaskSessionStateWaitingForInput}
	plainStep := &wfmodels.WorkflowStep{ID: "step-1", Name: "Step"}

	require.True(t, svc.shouldAutoResumeWithHandoff(ctx, waiting, plainStep, false))

	svc.config.ColdCacheAutoHandoff = false
	assert.False(t, svc.shouldAutoResumeWithHandoff(ctx, waiting, plainStep, false), "flag off")
	svc.config.ColdCacheAutoHandoff = true

	assert.False(t, svc.shouldAutoResumeWithHandoff(ctx, waiting, plainStep, true), "passthrough session")

	resetStep := &wfmodels.WorkflowStep{
		ID: "step-reset", Name: "Reset",
		Events: wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterResetAgentContext}}},
	}
	assert.False(t, svc.shouldAutoResumeWithHandoff(ctx, waiting, resetStep, false), "step declares reset_agent_context")

	running := &models.TaskSession{ID: "s2", State: models.TaskSessionStateRunning}
	assert.False(t, svc.shouldAutoResumeWithHandoff(ctx, running, plainStep, false), "a running turn must not be reset")

	svc.sessionlessRunner = nil
	assert.False(t, svc.shouldAutoResumeWithHandoff(ctx, waiting, plainStep, false), "no extraction runner")
}

// TestColdCacheAutoHandoff_FlagOffStepEntryUnchanged pins the I0 acceptance:
// with features.coldCacheAutoHandoff off, step entry keeps today's behavior —
// the automatic extraction is never selected, so an eligible cold session is
// prompted as-is.
func TestColdCacheAutoHandoff_FlagOffStepEntryUnchanged(t *testing.T) {
	ctx := context.Background()
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	svc.config.ColdCacheAutoHandoff = false
	runnerCalls := 0
	svc.sessionlessRunner = func(context.Context, string, string) (string, error) {
		runnerCalls++
		return "handoff", nil
	}
	old := time.Now().Add(-2 * time.Hour)
	svc.sessionUsageTotals = func(context.Context, string) (*models.TaskUsageTotals, error) {
		return &models.TaskUsageTotals{EventCount: 1, LastEventAt: &old}, nil
	}
	waiting := &models.TaskSession{ID: "s1", State: models.TaskSessionStateWaitingForInput}
	step := &wfmodels.WorkflowStep{ID: "step-1", Name: "Step"}

	assert.False(t, svc.shouldAutoResumeWithHandoff(ctx, waiting, step, false),
		"flag off must not select the automatic cold-cache continuation")
	assert.Equal(t, 0, runnerCalls, "flag off must not run the extraction runner")

	// Sanity: the same inputs select the continuation once the flag is on.
	svc.config.ColdCacheAutoHandoff = true
	assert.True(t, svc.shouldAutoResumeWithHandoff(ctx, waiting, step, false))
}
