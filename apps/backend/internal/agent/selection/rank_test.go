package selection

import (
	"math/rand/v2"
	"reflect"
	"testing"
)

func ids(ranked []RankedCandidate) []string {
	out := make([]string, 0, len(ranked))
	for _, candidate := range ranked {
		out = append(out, candidate.ProfileID)
	}
	return out
}

func candidate(id string, position int, tags []string, state QuotaState, remaining float64) RankCandidate {
	return RankCandidate{ProfileID: id, Position: position, Tags: tags, QuotaState: state, Remaining: remaining}
}

func TestRankCandidatesPrefersEqualCapacityPreferred(t *testing.T) {
	input := RankInput{
		Candidates: []RankCandidate{
			candidate("neutral", 0, []string{"codex"}, QuotaKnown, 50),
			candidate("preferred", 1, []string{"claude"}, QuotaKnown, 50),
		},
		Preferences: RankPreferences{PreferredTags: []string{"claude"}},
	}
	ranked := RankCandidates(input)
	if got := ids(ranked); !reflect.DeepEqual(got, []string{"preferred", "neutral"}) {
		t.Fatalf("order = %v", got)
	}
	if ranked[0].Reason != ReasonPreferredTagMatch {
		t.Fatalf("reason = %q, want %q", ranked[0].Reason, ReasonPreferredTagMatch)
	}
}

func TestRankCandidatesSkipsZeroPreferredForUsableFallback(t *testing.T) {
	input := RankInput{
		Candidates: []RankCandidate{
			candidate("neutral", 0, []string{"codex"}, QuotaKnown, 30),
		},
		Preferences:          RankPreferences{PreferredTags: []string{"claude"}},
		PreferredUnavailable: true,
	}
	ranked := RankCandidates(input)
	if got := ids(ranked); !reflect.DeepEqual(got, []string{"neutral"}) {
		t.Fatalf("order = %v", got)
	}
	if ranked[0].Reason != ReasonPreferredUnavailableFallback {
		t.Fatalf("reason = %q", ranked[0].Reason)
	}
}

func TestRankCandidatesAvoidedLosesToNeutralButWinsAlone(t *testing.T) {
	prefs := RankPreferences{AvoidedTags: []string{"gemini"}}
	withNeutral := RankCandidates(RankInput{
		Candidates: []RankCandidate{
			candidate("avoided", 0, []string{"gemini"}, QuotaKnown, 60),
			candidate("neutral", 1, []string{"codex"}, QuotaKnown, 60),
		},
		Preferences: prefs,
	})
	if got := ids(withNeutral); !reflect.DeepEqual(got, []string{"neutral", "avoided"}) {
		t.Fatalf("order = %v", got)
	}
	avoidedOnly := RankCandidates(RankInput{
		Candidates:  []RankCandidate{candidate("avoided", 0, []string{"gemini"}, QuotaKnown, 60)},
		Preferences: prefs,
	})
	if got := ids(avoidedOnly); !reflect.DeepEqual(got, []string{"avoided"}) {
		t.Fatalf("order = %v", got)
	}
	if avoidedOnly[0].Reason != ReasonAvoidedOnlyFallback {
		t.Fatalf("reason = %q", avoidedOnly[0].Reason)
	}
}

func TestRankCandidatesEmptyPreferencesUsesCapacityThenOrder(t *testing.T) {
	ranked := RankCandidates(RankInput{
		Candidates: []RankCandidate{
			candidate("first-low", 0, nil, QuotaKnown, 10),
			candidate("second-high", 1, nil, QuotaKnown, 80),
			candidate("unknown", 2, nil, QuotaUnknown, 0),
		},
	})
	if got := ids(ranked); !reflect.DeepEqual(got, []string{"second-high", "first-low", "unknown"}) {
		t.Fatalf("order = %v", got)
	}
	if ranked[0].Reason != ReasonQuotaHeadroom {
		t.Fatalf("positive reason = %q", ranked[0].Reason)
	}
	if ranked[2].Reason != ReasonConfiguredOrder {
		t.Fatalf("unknown reason = %q", ranked[2].Reason)
	}
}

func TestRankCandidatesOnlyNonPreferredAvailableSelects(t *testing.T) {
	ranked := RankCandidates(RankInput{
		Candidates: []RankCandidate{
			candidate("neutral", 0, []string{"codex"}, QuotaKnown, 5),
		},
		Preferences: RankPreferences{PreferredTags: []string{"claude"}},
	})
	if got := ids(ranked); !reflect.DeepEqual(got, []string{"neutral"}) {
		t.Fatalf("order = %v", got)
	}
	if ranked[0].Reason != ReasonQuotaHeadroom {
		t.Fatalf("reason = %q", ranked[0].Reason)
	}
}

func TestRankCandidatesSeededRandDeterministic(t *testing.T) {
	newRand := func() func() float64 {
		rng := rand.New(rand.NewPCG(42, 100))
		return rng.Float64
	}
	input := func() RankInput {
		return RankInput{
			Candidates: []RankCandidate{
				candidate("a", 0, nil, QuotaKnown, 50),
				candidate("b", 1, nil, QuotaKnown, 50),
				candidate("c", 2, nil, QuotaKnown, 50),
			},
			Rand: newRand(),
		}
	}
	run1 := ids(RankCandidates(input()))
	run2 := ids(RankCandidates(input()))
	if !reflect.DeepEqual(run1, run2) {
		t.Fatalf("seeded runs differed: run1=%v, run2=%v", run1, run2)
	}
}

func TestRankCandidatesWeightedSamplingRatio(t *testing.T) {
	rng := rand.New(rand.NewPCG(12345, 67890))
	const iterations = 10000
	wins := make(map[string]int)
	for i := 0; i < iterations; i++ {
		ranked := RankCandidates(RankInput{
			Candidates: []RankCandidate{
				candidate("cand-60", 0, nil, QuotaKnown, 60),
				candidate("cand-30", 1, nil, QuotaKnown, 30),
			},
			Rand: rng.Float64,
		})
		wins[ranked[0].ProfileID]++
	}
	ratio := float64(wins["cand-60"]) / float64(wins["cand-30"])
	if ratio < 1.8 || ratio > 2.2 {
		t.Fatalf("ratio = %f (60: %d, 30: %d), want ~2.0", ratio, wins["cand-60"], wins["cand-30"])
	}
}

func TestRankCandidatesPreferredAlwaysBeatsNeutralRegardlessOfRand(t *testing.T) {
	rng := rand.New(rand.NewPCG(999, 888))
	for i := 0; i < 200; i++ {
		ranked := RankCandidates(RankInput{
			Candidates: []RankCandidate{
				candidate("preferred", 0, []string{"claude"}, QuotaKnown, 10),
				candidate("neutral", 1, []string{"codex"}, QuotaKnown, 90),
			},
			Preferences: RankPreferences{PreferredTags: []string{"claude"}},
			Rand:        rng.Float64,
		})
		if ranked[0].ProfileID != "preferred" {
			t.Fatalf("neutral beat preferred: %v", ids(ranked))
		}
	}
}

func TestRankCandidatesUnknownBandOrderUnchanged(t *testing.T) {
	rng := rand.New(rand.NewPCG(111, 222))
	ranked := RankCandidates(RankInput{
		Candidates: []RankCandidate{
			candidate("pos-1", 0, nil, QuotaKnown, 50),
			candidate("unk-first", 1, nil, QuotaUnknown, 0),
			candidate("unk-second", 2, nil, QuotaUnknown, 0),
		},
		Rand: rng.Float64,
	})
	got := ids(ranked)
	if got[0] != "pos-1" || got[1] != "unk-first" || got[2] != "unk-second" {
		t.Fatalf("unexpected order = %v", got)
	}
}

func TestRankCandidatesNilRandMatchesLegacyOrder(t *testing.T) {
	ranked := RankCandidates(RankInput{
		Candidates: []RankCandidate{
			candidate("low", 0, nil, QuotaKnown, 20),
			candidate("high", 1, nil, QuotaKnown, 80),
		},
		Rand: nil,
	})
	if got := ids(ranked); !reflect.DeepEqual(got, []string{"high", "low"}) {
		t.Fatalf("order = %v, want [high low]", got)
	}
}
