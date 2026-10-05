package orchestrator

import (
	"context"

	"github.com/kandev/kandev/internal/task/models"
)

type profileQuotaChecker interface {
	ProfileQuotaExhausted(ctx context.Context, profileID string) bool
}

// sessionTargetProfileID returns the execution profile ID if present, else the agent profile ID.
func sessionTargetProfileID(session *models.TaskSession) string {
	if session == nil {
		return ""
	}
	if session.ExecutionProfileID != "" {
		return session.ExecutionProfileID
	}
	return session.AgentProfileID
}

// newProfileAvailabilityMemo returns a memoized function reporting whether a profile's quota is exhausted.
// It caches results per profile ID for the lifetime of the returned function.
func (s *Service) newProfileAvailabilityMemo(ctx context.Context) func(profileID string) bool {
	var checker profileQuotaChecker
	if s.profileExecutionResolver != nil {
		checker = s.profileExecutionResolver
	}
	return newProfileAvailabilityMemoFromChecker(ctx, checker)
}

func newProfileAvailabilityMemoFromChecker(ctx context.Context, checker profileQuotaChecker) func(profileID string) bool {
	if checker == nil {
		return func(string) bool { return false }
	}
	memo := make(map[string]bool)
	return func(profileID string) bool {
		if profileID == "" {
			return false
		}
		if exhausted, ok := memo[profileID]; ok {
			return exhausted
		}
		exhausted := checker.ProfileQuotaExhausted(ctx, profileID)
		memo[profileID] = exhausted
		return exhausted
	}
}
