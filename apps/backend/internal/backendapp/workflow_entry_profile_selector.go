package backendapp

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
	agentregistry "github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/selection"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// workflowEntryProfileSelector resolves the concrete agent profile to freeze
// for one workflow entry. Candidate listing and eligibility stay here; ranking
// and scoring live in internal/agent/selection.
type workflowEntryProfileSelector struct {
	settingsStore settingsstore.Repository
	agentRegistry *agentregistry.Registry
	strategy      *selection.QuotaStrategy
	logger        *logger.Logger
	validator     taskAgentExecutorCompatibilityValidator
}

func newWorkflowEntryProfileSelector(
	settingsStore settingsstore.Repository,
	agentRegistry *agentregistry.Registry,
	usageAdapter *usageProviderAdapter,
	log *logger.Logger,
) *workflowEntryProfileSelector {
	return &workflowEntryProfileSelector{
		settingsStore: settingsStore,
		agentRegistry: agentRegistry,
		strategy:      &selection.QuotaStrategy{Provider: usageAdapter},
		logger:        log,
		validator:     taskAgentExecutorCompatibilityValidator{profiles: settingsStore, agentRegistry: agentRegistry},
	}
}

// SelectEntryProfile implements taskservice.WorkflowEntryProfileSelector.
func (s *workflowEntryProfileSelector) SelectEntryProfile(
	ctx context.Context,
	stepID string,
	allowedTags []string,
	fallbackProfileID string,
	executor *taskmodels.Executor,
	executorProfile *taskmodels.ExecutorProfile,
) (string, error) {
	candidates, err := s.eligibleCandidates(ctx, allowedTags, executor, executorProfile)
	if err != nil {
		return "", err
	}
	matched := selection.FilterCandidates(candidates, allowedTags)
	if len(matched) == 0 {
		return s.finishSelection(stepID, fallbackProfileID, "no_tag_match", "static_fallback", len(matched), "unknown", nil)
	}
	decision, err := s.strategy.Select(ctx, matched)
	if err != nil {
		return s.finishSelection(stepID, fallbackProfileID, "no_candidates", "static_fallback", len(matched), "unknown", nil)
	}
	if decision.AllExhausted {
		return s.finishSelection(stepID, fallbackProfileID, "all_exhausted", "static_fallback", len(matched), string(decision.QuotaState), matched)
	}
	s.logSelection(stepID, decision.ProfileID, decision.Strategy, decision.Reason, len(matched), string(decision.QuotaState))
	return decision.ProfileID, nil
}

// finishSelection logs a fallback decision and returns the safe static
// fallback, or the typed no-eligible error when none exists.
func (s *workflowEntryProfileSelector) finishSelection(
	stepID, fallbackProfileID, reason, strategy string,
	candidateCount int,
	quotaState string,
	exhausted []selection.Candidate,
) (string, error) {
	chosen, err := staticFallback(fallbackProfileID, exhausted)
	if err != nil {
		s.logSelection(stepID, "", strategy, reason, candidateCount, quotaState)
		return "", err
	}
	s.logSelection(stepID, chosen, strategy, reason, candidateCount, quotaState)
	return chosen, nil
}

// logSelection emits one structured selection log. It never records
// credentials, credential paths, account IDs, or raw provider payloads.
func (s *workflowEntryProfileSelector) logSelection(stepID, chosenProfileID, strategy, reason string, candidateCount int, quotaState string) {
	if s.logger == nil {
		return
	}
	s.logger.Info("workflow entry profile selected",
		zap.String("step_id", stepID),
		zap.String("agent_profile_id", chosenProfileID),
		zap.String("strategy", strategy),
		zap.String("reason", reason),
		zap.Int("candidate_count", candidateCount),
		zap.String("quota_state", quotaState))
}

// eligibleCandidates returns the global, enabled, concrete, launchable profiles
// whose tags intersect allowedTags. A profile that the task's executor cannot
// run is excluded before ranking.
func (s *workflowEntryProfileSelector) eligibleCandidates(
	ctx context.Context,
	allowedTags []string,
	executor *taskmodels.Executor,
	executorProfile *taskmodels.ExecutorProfile,
) ([]selection.Candidate, error) {
	if s.settingsStore == nil || len(allowedTags) == 0 {
		return nil, nil
	}
	agentRows, err := s.settingsStore.ListAgents(ctx)
	if err != nil {
		return nil, fmt.Errorf("list agents for workflow selection: %w", err)
	}
	candidates := make([]selection.Candidate, 0)
	for _, agentRow := range agentRows {
		if agentRow == nil || !s.agentLaunchable(agentRow.Name) {
			continue
		}
		profiles, err := s.settingsStore.ListAgentProfiles(ctx, agentRow.ID)
		if err != nil {
			return nil, fmt.Errorf("list profiles for workflow selection: %w", err)
		}
		for _, profile := range profiles {
			if !eligibleConcreteProfile(profile) {
				continue
			}
			if !s.executorCompatible(ctx, profile, executor, executorProfile) {
				continue
			}
			candidates = append(candidates, selection.Candidate{
				ProfileID: profile.ID,
				Tags:      profile.Tags,
			})
		}
	}
	return candidates, nil
}

// executorCompatible reuses the task executor compatibility validator. It is
// permissive when no executor is known, so a selection never fails solely
// because executor context was unavailable.
func (s *workflowEntryProfileSelector) executorCompatible(
	ctx context.Context,
	profile *settingsmodels.AgentProfile,
	executor *taskmodels.Executor,
	executorProfile *taskmodels.ExecutorProfile,
) bool {
	if executor == nil {
		return true
	}
	if err := s.validator.ValidateAgentProfileForExecutor(ctx, profile, executor, executorProfile); err != nil {
		return false
	}
	return true
}

func (s *workflowEntryProfileSelector) agentLaunchable(agentName string) bool {
	if s.agentRegistry == nil {
		return true
	}
	agent, ok := s.agentRegistry.Get(agentName)
	return ok && agent.Enabled()
}

func eligibleConcreteProfile(profile *settingsmodels.AgentProfile) bool {
	return profile != nil &&
		profile.Enabled &&
		profile.WorkspaceID == "" &&
		profile.Role == "" &&
		profile.AgentID != agents.DynamicAgentID
}

// staticFallback returns the explicit fallback unless it is one of the
// exhausted tagged candidates, in which case no safe choice remains.
func staticFallback(fallbackProfileID string, exhausted []selection.Candidate) (string, error) {
	if fallbackProfileID == "" {
		return "", taskservice.ErrNoEligibleEntryProfile
	}
	for _, candidate := range exhausted {
		if candidate.ProfileID == fallbackProfileID {
			return "", taskservice.ErrNoEligibleEntryProfile
		}
	}
	return fallbackProfileID, nil
}
