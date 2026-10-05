package selection

import (
	"context"
	"errors"
	"testing"

	agentusage "github.com/kandev/kandev/internal/agent/usage"
)

type fakeUsage struct {
	usage map[string]*agentusage.ProviderUsage
	errs  map[string]error
}

func (f fakeUsage) GetUsage(_ context.Context, profileID string) (*agentusage.ProviderUsage, error) {
	if err := f.errs[profileID]; err != nil {
		return nil, err
	}
	return f.usage[profileID], nil
}

func usageAt(pct float64) *agentusage.ProviderUsage {
	return &agentusage.ProviderUsage{Windows: []agentusage.UtilizationWindow{{UtilizationPct: pct}}}
}

func TestMatchesOrSemantics(t *testing.T) {
	if !Matches([]string{"review", "security"}, []string{"security"}) {
		t.Fatal("expected OR match on second tag")
	}
	if Matches([]string{"review"}, []string{"security"}) {
		t.Fatal("unexpected match")
	}
	if Matches([]string{"review"}, nil) {
		t.Fatal("empty allowed tags must not match")
	}
}

func TestFilterCandidatesMultiTagIsOneCandidate(t *testing.T) {
	candidates := []Candidate{
		{ProfileID: "a", Tags: []string{"review", "security"}},
		{ProfileID: "b", Tags: []string{"ops"}},
	}
	matched := FilterCandidates(candidates, []string{"review", "security"})
	if len(matched) != 1 || matched[0].ProfileID != "a" {
		t.Fatalf("matched = %#v, want only a", matched)
	}
}

func TestQuotaStrategyHighestRemaining(t *testing.T) {
	strategy := &QuotaStrategy{Provider: fakeUsage{usage: map[string]*agentusage.ProviderUsage{
		"a": usageAt(64),
		"b": usageAt(28),
	}}}
	decision, err := strategy.Select(context.Background(), []Candidate{{ProfileID: "a"}, {ProfileID: "b"}})
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if decision.ProfileID != "b" || decision.QuotaState != QuotaKnown {
		t.Fatalf("decision = %#v, want b known", decision)
	}
}

func TestQuotaStrategyIgnoresOtherModelsQuota(t *testing.T) {
	strategy := &QuotaStrategy{Provider: fakeUsage{usage: map[string]*agentusage.ProviderUsage{
		"pro":   {Windows: []agentusage.UtilizationWindow{{Model: "gemini-pro", UtilizationPct: 100}}},
		"flash": {Windows: []agentusage.UtilizationWindow{{Model: "gemini-pro", UtilizationPct: 100}, {Model: "gemini-flash", UtilizationPct: 20}}},
	}}}
	decision, err := strategy.Select(context.Background(), []Candidate{{ProfileID: "pro", Model: "gemini-pro"}, {ProfileID: "flash", Model: "gemini-flash"}})
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if decision.ProfileID != "flash" {
		t.Fatalf("decision = %#v, want flash", decision)
	}
}

func TestQuotaStrategyTieBreaksByProfileID(t *testing.T) {
	strategy := &QuotaStrategy{Provider: fakeUsage{usage: map[string]*agentusage.ProviderUsage{
		"z": usageAt(50),
		"a": usageAt(50),
	}}}
	decision, err := strategy.Select(context.Background(), []Candidate{{ProfileID: "z"}, {ProfileID: "a"}})
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if decision.ProfileID != "a" {
		t.Fatalf("decision = %#v, want a", decision)
	}
}

func TestQuotaStrategyKnownPositiveOutranksUnknown(t *testing.T) {
	strategy := &QuotaStrategy{Provider: fakeUsage{usage: map[string]*agentusage.ProviderUsage{
		"a": usageAt(99),
	}}}
	decision, err := strategy.Select(context.Background(), []Candidate{{ProfileID: "a"}, {ProfileID: "b"}})
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if decision.ProfileID != "a" {
		t.Fatalf("decision = %#v, want a (known positive quota)", decision)
	}
}

func TestQuotaStrategyUnknownOutranksZero(t *testing.T) {
	strategy := &QuotaStrategy{Provider: fakeUsage{usage: map[string]*agentusage.ProviderUsage{
		"a": usageAt(100),
	}}}
	decision, err := strategy.Select(context.Background(), []Candidate{{ProfileID: "a"}, {ProfileID: "b"}})
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if decision.ProfileID != "b" || decision.QuotaState != QuotaUnknown {
		t.Fatalf("decision = %#v, want unknown b", decision)
	}
}

func TestQuotaStrategyFetchErrorIsCandidateLocal(t *testing.T) {
	strategy := &QuotaStrategy{Provider: fakeUsage{
		usage: map[string]*agentusage.ProviderUsage{"a": usageAt(10)},
		errs:  map[string]error{"b": errors.New("boom")},
	}}
	decision, err := strategy.Select(context.Background(), []Candidate{{ProfileID: "a"}, {ProfileID: "b"}})
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if decision.ProfileID != "a" {
		t.Fatalf("decision = %#v, want a despite b error", decision)
	}
}

func TestQuotaStrategyAllExhausted(t *testing.T) {
	strategy := &QuotaStrategy{Provider: fakeUsage{usage: map[string]*agentusage.ProviderUsage{
		"a": usageAt(100),
		"b": usageAt(100),
	}}}
	decision, err := strategy.Select(context.Background(), []Candidate{{ProfileID: "b"}, {ProfileID: "a"}})
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if !decision.AllExhausted || decision.ProfileID != "a" {
		t.Fatalf("decision = %#v, want all-exhausted a", decision)
	}
}

func TestQuotaStrategyNoCandidates(t *testing.T) {
	if _, err := (&QuotaStrategy{}).Select(context.Background(), nil); !errors.Is(err, ErrNoEligibleProfile) {
		t.Fatalf("err = %v, want ErrNoEligibleProfile", err)
	}
}
