// Package selection chooses one eligible concrete agent profile for a
// tag-configured workflow step. Tag filtering and fallback belong to the
// caller; this package owns candidate ranking and the strategy boundary.
package selection

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"

	agentusage "github.com/kandev/kandev/internal/agent/usage"
)

// QuotaState classifies a candidate's subscription telemetry.
type QuotaState string

const (
	QuotaKnown       QuotaState = "known"
	QuotaUnknown     QuotaState = "unknown"
	QuotaUnavailable QuotaState = "unavailable"
)

// Candidate is an eligible concrete agent profile and its canonical tags.
type Candidate struct {
	ProfileID string
	Tags      []string
}

// Decision is the result of one selection over a candidate set.
type Decision struct {
	ProfileID  string
	Strategy   string
	Reason     string
	QuotaState QuotaState
	// AllExhausted is true when every candidate had a known-zero remaining
	// quota. The caller then applies its static fallback (if any).
	AllExhausted bool
}

// ErrNoEligibleProfile is returned when the candidate set is empty.
var ErrNoEligibleProfile = errors.New("no eligible agent profile")

// UsageProvider fetches subscription utilization for one profile. A provider
// fetch error is candidate-local; it never fails the whole selection.
type UsageProvider interface {
	GetUsage(ctx context.Context, profileID string) (*agentusage.ProviderUsage, error)
}

// Strategy ranks a candidate set and chooses one profile.
type Strategy interface {
	Select(ctx context.Context, candidates []Candidate) (Decision, error)
}

// Matches reports whether profileTags intersects allowedTags (OR semantics).
// An empty allowedTags list matches nothing; callers treat that as
// "tags disabled" before calling.
func Matches(profileTags, allowedTags []string) bool {
	if len(allowedTags) == 0 {
		return false
	}
	allowed := make(map[string]struct{}, len(allowedTags))
	for _, tag := range allowedTags {
		allowed[strings.ToLower(strings.TrimSpace(tag))] = struct{}{}
	}
	for _, tag := range profileTags {
		if _, ok := allowed[strings.ToLower(strings.TrimSpace(tag))]; ok {
			return true
		}
	}
	return false
}

// FilterCandidates returns the candidates that match at least one allowed tag.
func FilterCandidates(candidates []Candidate, allowedTags []string) []Candidate {
	if len(allowedTags) == 0 {
		return nil
	}
	matched := make([]Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		if Matches(candidate.Tags, allowedTags) {
			matched = append(matched, candidate)
		}
	}
	return matched
}

// QuotaStrategy selects the candidate with the most remaining subscription
// quota. Known-positive quota outranks unknown/unavailable, which outranks
// known-zero. Ties break on canonical profile ID ascending.
type QuotaStrategy struct {
	Provider UsageProvider
}

type ranked struct {
	profileID string
	state     QuotaState
	remaining float64
}

// Select ranks the candidates and returns the best eligible profile.
func (s *QuotaStrategy) Select(ctx context.Context, candidates []Candidate) (Decision, error) {
	rankedCandidates := s.rank(ctx, candidates)
	if len(rankedCandidates) == 0 {
		return Decision{}, ErrNoEligibleProfile
	}

	positives := make([]ranked, 0, len(rankedCandidates))
	unknowns := make([]ranked, 0, len(rankedCandidates))
	zeros := make([]ranked, 0, len(rankedCandidates))
	for _, candidate := range rankedCandidates {
		switch {
		case candidate.state == QuotaKnown && candidate.remaining > 0:
			positives = append(positives, candidate)
		case candidate.state == QuotaKnown:
			zeros = append(zeros, candidate)
		default:
			unknowns = append(unknowns, candidate)
		}
	}

	if len(positives) > 0 {
		sort.Slice(positives, func(i, j int) bool {
			if positives[i].remaining != positives[j].remaining {
				return positives[i].remaining > positives[j].remaining
			}
			return positives[i].profileID < positives[j].profileID
		})
		best := positives[0]
		return Decision{
			ProfileID:  best.profileID,
			Strategy:   "quota",
			Reason:     "highest_remaining_quota",
			QuotaState: QuotaKnown,
		}, nil
	}

	if len(unknowns) > 0 {
		sort.Slice(unknowns, func(i, j int) bool { return unknowns[i].profileID < unknowns[j].profileID })
		best := unknowns[0]
		return Decision{
			ProfileID:  best.profileID,
			Strategy:   "quota",
			Reason:     "unknown_quota",
			QuotaState: best.state,
		}, nil
	}

	// Every candidate is known-zero.
	sort.Slice(zeros, func(i, j int) bool { return zeros[i].profileID < zeros[j].profileID })
	return Decision{
		ProfileID:    zeros[0].profileID,
		Strategy:     "quota",
		Reason:       "all_exhausted",
		QuotaState:   QuotaKnown,
		AllExhausted: true,
	}, nil
}

// rank scores every candidate concurrently. A provider error or absent
// telemetry degrades only that candidate.
func (s *QuotaStrategy) rank(ctx context.Context, candidates []Candidate) []ranked {
	if len(candidates) == 0 {
		return nil
	}
	results := make([]ranked, len(candidates))
	var wg sync.WaitGroup
	for i, candidate := range candidates {
		wg.Add(1)
		go func(i int, candidate Candidate) {
			defer wg.Done()
			results[i] = s.score(ctx, candidate.ProfileID)
		}(i, candidate)
	}
	wg.Wait()
	return results
}

func (s *QuotaStrategy) score(ctx context.Context, profileID string) ranked {
	result := ranked{profileID: profileID, state: QuotaUnknown}
	if s.Provider == nil {
		return result
	}
	usage, err := s.Provider.GetUsage(ctx, profileID)
	if err != nil {
		result.state = QuotaUnavailable
		return result
	}
	remaining, known := agentusage.RemainingPct(usage)
	if !known {
		return result
	}
	result.state = QuotaKnown
	result.remaining = remaining
	return result
}
