package providerusage

import (
	"testing"
	"time"
)

func ptr(v float64) *float64 { return &v }

func TestMeasuredLabelsKeepsNewestReading(t *testing.T) {
	reset := time.Date(2026, 9, 25, 15, 0, 0, 0, time.UTC)
	observations := []Observation{
		{Kind: KindMeasured, WindowLabel: "5-hour", UtilizationPct: ptr(10), ResetAt: reset, ObservedAt: reset.Add(-5 * time.Hour)},
		{Kind: KindMeasured, WindowLabel: "5-hour", UtilizationPct: ptr(50), ResetAt: reset, ObservedAt: reset.Add(-4 * time.Hour)},
	}
	windows := measuredLabels(&accountAccumulator{}, observations)
	if got := windows["5-hour"].UtilizationPct; got != 50 {
		t.Fatalf("expected the newest reading (50), got %v", got)
	}
}

func TestActiveLimitHitExpiresWithoutReset(t *testing.T) {
	now := time.Now().UTC()
	fresh := Observation{Kind: KindLimitHit, ObservedAt: now.Add(-30 * time.Minute)}
	stale := Observation{Kind: KindLimitHit, ObservedAt: now.Add(-2 * time.Hour)}
	futureReset := Observation{Kind: KindLimitHit, ObservedAt: now.Add(-2 * time.Hour), ResetAt: now.Add(time.Hour)}
	pastReset := Observation{Kind: KindLimitHit, ObservedAt: now.Add(-30 * time.Minute), ResetAt: now.Add(-time.Minute)}

	if !activeLimitHit([]Observation{fresh}, now) {
		t.Fatal("a fresh hit with no reset hint must be active")
	}
	if activeLimitHit([]Observation{stale}, now) {
		t.Fatal("a hit with no reset hint older than the TTL must expire")
	}
	if !activeLimitHit([]Observation{futureReset}, now) {
		t.Fatal("a hit with a future reset must be active")
	}
	if activeLimitHit([]Observation{pastReset}, now) {
		t.Fatal("a hit with a past reset must expire")
	}
}
