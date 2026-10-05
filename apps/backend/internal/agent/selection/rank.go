package selection

import (
	"math"
	"sort"
)

// Selection reasons persisted with a dynamic route decision. The vocabulary is
// closed so the session projection and route-attempt history stay explainable.
const (
	ReasonPreferredTagMatch            = "preferred_tag_match"
	ReasonQuotaHeadroom                = "quota_headroom"
	ReasonConfiguredOrder              = "configured_order"
	ReasonPreferredUnavailableFallback = "preferred_unavailable_fallback"
	ReasonAvoidedOnlyFallback          = "avoided_only_fallback"
)

// RankCandidate is one eligible dynamic-profile candidate with the bounded
// telemetry used for schedule-time ranking. Position is the configured
// candidate position and is the final tie-break.
type RankCandidate struct {
	ProfileID  string
	Position   int
	Tags       []string
	QuotaState QuotaState
	Remaining  float64
}

// RankPreferences are the dynamic profile's soft tag preferences.
type RankPreferences struct {
	PreferredTags []string
	AvoidedTags   []string
}

// RankedCandidate is one candidate in ranked order with its explainable reason.
type RankedCandidate struct {
	ProfileID string
	Reason    string
}

// RankInput is one ranking pass. PreferredUnavailable reports that a
// preferred-tag candidate exists in the profile's full candidate list but is
// not eligible for selection (for example, known-zero capacity), so a neutral
// or avoided fallback is explained as such.
// When Rand is non-nil, candidates within each contiguous band of positive-capacity,
// same-affinity candidates are reordered by weighted sampling without replacement.
type RankInput struct {
	Candidates           []RankCandidate
	Preferences          RankPreferences
	PreferredUnavailable bool
	Rand                 func() float64
}

type affinity int

const (
	affinityPreferred affinity = iota
	affinityNeutral
	affinityAvoided
)

func tagAffinity(tags []string, preferences RankPreferences) affinity {
	tagSet := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		tagSet[tag] = struct{}{}
	}
	for _, tag := range preferences.PreferredTags {
		if _, ok := tagSet[tag]; ok {
			return affinityPreferred
		}
	}
	for _, tag := range preferences.AvoidedTags {
		if _, ok := tagSet[tag]; ok {
			return affinityAvoided
		}
	}
	return affinityNeutral
}

// isPositiveCapacity reports whether a candidate has known, non-zero capacity.
func isPositiveCapacity(candidate RankCandidate) bool {
	return candidate.QuotaState == QuotaKnown && candidate.Remaining > 0
}

func candidateReason(candidate RankCandidate, input RankInput) string {
	switch tagAffinity(candidate.Tags, input.Preferences) {
	case affinityPreferred:
		return ReasonPreferredTagMatch
	case affinityAvoided:
		return ReasonAvoidedOnlyFallback
	default:
		if input.PreferredUnavailable {
			return ReasonPreferredUnavailableFallback
		}
		if isPositiveCapacity(candidate) {
			return ReasonQuotaHeadroom
		}
		return ReasonConfiguredOrder
	}
}

// RankCandidates orders eligible candidates by capacity band, soft tag
// affinity, remaining percentage, then configured position, and assigns each
// one a closed selection reason. Known-positive capacity outranks unknown or
// unavailable; within a band preferred tags outrank neutral, which outrank
// avoided. A preferred-tag candidate with no usable capacity never blocks a
// neutral or avoided fallback. When input.Rand is provided, each contiguous run
// of known-positive candidates with the same tag affinity is reordered by
// weighted sampling without replacement using remaining percentage as weight.
// A stable sort preserves configured order when every ranking key ties.
func RankCandidates(input RankInput) []RankedCandidate {
	ordered := make([]RankCandidate, len(input.Candidates))
	copy(ordered, input.Candidates)
	sort.SliceStable(ordered, func(i, j int) bool {
		left, right := ordered[i], ordered[j]
		leftPositive, rightPositive := isPositiveCapacity(left), isPositiveCapacity(right)
		if leftPositive != rightPositive {
			return leftPositive
		}
		leftAffinity := tagAffinity(left.Tags, input.Preferences)
		rightAffinity := tagAffinity(right.Tags, input.Preferences)
		if leftAffinity != rightAffinity {
			return leftAffinity < rightAffinity
		}
		if left.Remaining != right.Remaining {
			return left.Remaining > right.Remaining
		}
		return left.Position < right.Position
	})
	if input.Rand != nil {
		n := len(ordered)
		i := 0
		for i < n {
			if !isPositiveCapacity(ordered[i]) {
				break
			}
			aff := tagAffinity(ordered[i].Tags, input.Preferences)
			j := i + 1
			for j < n && isPositiveCapacity(ordered[j]) && tagAffinity(ordered[j].Tags, input.Preferences) == aff {
				j++
			}
			if j-i > 1 {
				reorderWeightedRun(ordered[i:j], input.Rand)
			}
			i = j
		}
	}
	ranked := make([]RankedCandidate, 0, len(ordered))
	for _, candidate := range ordered {
		ranked = append(ranked, RankedCandidate{
			ProfileID: candidate.ProfileID,
			Reason:    candidateReason(candidate, input),
		})
	}
	return ranked
}

type keyedCandidate struct {
	candidate RankCandidate
	key       float64
}

func reorderWeightedRun(run []RankCandidate, randFn func() float64) {
	keyed := make([]keyedCandidate, len(run))
	for idx, c := range run {
		u := randFn()
		if u <= 0 {
			u = 1e-15
		}
		key := math.Pow(u, 1.0/c.Remaining)
		keyed[idx] = keyedCandidate{candidate: c, key: key}
	}
	sort.SliceStable(keyed, func(i, j int) bool {
		if keyed[i].key != keyed[j].key {
			return keyed[i].key > keyed[j].key
		}
		return keyed[i].candidate.Position < keyed[j].candidate.Position
	})
	for idx := range run {
		run[idx] = keyed[idx].candidate
	}
}
