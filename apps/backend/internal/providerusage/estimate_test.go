package providerusage

import (
	"testing"
	"time"
)

func TestEstimateWindow(t *testing.T) {
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	base := now
	series := func(utils []float64, step time.Duration) []Point {
		points := make([]Point, 0, len(utils))
		for i, util := range utils {
			points = append(points, Point{
				UtilPct: util,
				At:      base.Add(time.Duration(i) * step),
				ResetAt: base.Add(4 * time.Hour),
				Kind:    KindMeasured,
			})
		}
		return points
	}

	tests := []struct {
		name      string
		points    []Point
		wantTrend string
		wantConf  string
		wantAnom  bool
	}{
		{name: "empty", points: nil, wantTrend: TrendFlat, wantConf: ConfidenceNone},
		{name: "single", points: series([]float64{10}, time.Minute), wantTrend: TrendFlat, wantConf: ConfidenceNone},
		{
			name:      "flat",
			points:    series([]float64{10, 10, 10, 10, 10, 10}, 20*time.Minute),
			wantTrend: TrendFlat, wantConf: ConfidenceMedium,
		},
		{
			name:      "rising",
			points:    series([]float64{10, 20, 30, 40, 50, 60}, 20*time.Minute),
			wantTrend: TrendRising, wantConf: ConfidenceMedium,
		},
		{
			name:      "falling",
			points:    series([]float64{60, 50, 40, 30, 20, 10}, 20*time.Minute),
			wantTrend: TrendFalling, wantConf: ConfidenceMedium,
		},
		{
			name:      "anomaly",
			points:    series([]float64{10, 10.05, 10.1, 10.15, 10.2, 10.25, 10.3, 25}, 20*time.Minute),
			wantTrend: TrendRising, wantConf: ConfidenceMedium, wantAnom: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := EstimateWindow(tc.points, now)
			if got.Trend != tc.wantTrend {
				t.Errorf("trend = %q, want %q", got.Trend, tc.wantTrend)
			}
			if got.Confidence != tc.wantConf {
				t.Errorf("confidence = %q, want %q", got.Confidence, tc.wantConf)
			}
			if got.Anomaly != tc.wantAnom {
				t.Errorf("anomaly = %v, want %v", got.Anomaly, tc.wantAnom)
			}
			if got.Source != "estimated" {
				t.Errorf("source = %q, want estimated", got.Source)
			}
		})
	}
}

func TestEstimateWindowResetBoundary(t *testing.T) {
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	oldReset := now.Add(-time.Hour)
	newReset := now.Add(4 * time.Hour)
	points := []Point{
		{UtilPct: 90, At: now.Add(-50 * time.Minute), ResetAt: oldReset, Kind: KindMeasured},
		{UtilPct: 95, At: now.Add(-40 * time.Minute), ResetAt: oldReset, Kind: KindMeasured},
		{UtilPct: 5, At: now.Add(-30 * time.Minute), ResetAt: newReset, Kind: KindMeasured},
		{UtilPct: 10, At: now.Add(-20 * time.Minute), ResetAt: newReset, Kind: KindMeasured},
		{UtilPct: 15, At: now.Add(-10 * time.Minute), ResetAt: newReset, Kind: KindMeasured},
	}
	got := EstimateWindow(points, now)
	if got.Trend != TrendRising {
		t.Fatalf("expected rising within the new window, got %+v", got)
	}
	if got.ResetAt != newReset {
		t.Fatalf("expected newest reset, got %v", got.ResetAt)
	}
}

func TestEstimateWindowLastDropWithoutReset(t *testing.T) {
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	points := []Point{
		{UtilPct: 80, At: now.Add(-50 * time.Minute), Kind: KindMeasured},
		{UtilPct: 90, At: now.Add(-40 * time.Minute), Kind: KindMeasured},
		{UtilPct: 5, At: now.Add(-30 * time.Minute), Kind: KindMeasured},
		{UtilPct: 10, At: now.Add(-20 * time.Minute), Kind: KindMeasured},
		{UtilPct: 15, At: now.Add(-10 * time.Minute), Kind: KindMeasured},
	}
	got := EstimateWindow(points, now)
	if got.Trend != TrendRising {
		t.Fatalf("expected rising after the drop, got %+v", got)
	}
}

func TestCalibrationZeroTokens(t *testing.T) {
	if factor := Calibration(50, 0); factor != 0 {
		t.Fatalf("expected zero factor, got %v", factor)
	}
	if series := EstimatedSeries([]TokenBucket{{At: time.Now(), WeightedTokens: 10}}, 0); series != nil {
		t.Fatalf("expected nil series without calibration, got %+v", series)
	}
}

func TestEstimatedSeriesCumulative(t *testing.T) {
	base := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	buckets := []TokenBucket{
		{At: base.Add(2 * time.Hour), WeightedTokens: 100},
		{At: base, WeightedTokens: 100},
	}
	series := EstimatedSeries(buckets, 0.1)
	if len(series) != 2 {
		t.Fatalf("expected 2 points, got %d", len(series))
	}
	if series[0].UtilPct != 10 || series[1].UtilPct != 20 {
		t.Fatalf("expected cumulative 10 then 20, got %+v", series)
	}
	if series[0].Kind != "estimated" {
		t.Fatalf("expected estimated kind, got %q", series[0].Kind)
	}
}

func TestAccountStatus(t *testing.T) {
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		in   StatusInput
		want string
	}{
		{name: "no source", in: StatusInput{Now: now}, want: StatusUnknown},
		{
			name: "exhausted by window",
			in:   StatusInput{Windows: []WindowUtil{{UtilizationPct: 100}}, Now: now},
			want: StatusExhausted,
		},
		{
			name: "nearing",
			in:   StatusInput{Windows: []WindowUtil{{UtilizationPct: 85}}, Now: now},
			want: StatusNearing,
		},
		{
			name: "healthy",
			in:   StatusInput{Windows: []WindowUtil{{UtilizationPct: 20}}, Now: now},
			want: StatusHealthy,
		},
		{
			name: "stale",
			in: StatusInput{
				Windows: []WindowUtil{{UtilizationPct: 20}}, LiveFetchFailed: true,
				LastObservedAt: now.Add(-time.Hour), Now: now,
			},
			want: StatusStale,
		},
		{
			name: "unavailable without history",
			in:   StatusInput{LiveFetchFailed: true, Now: now},
			want: StatusUnavailable,
		},
		{name: "limit hit", in: StatusInput{HasLimitHit: true, Now: now}, want: StatusExhausted},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := AccountStatus(tc.in); got != tc.want {
				t.Fatalf("status = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestWorstStatus(t *testing.T) {
	if got := WorstStatus([]string{StatusHealthy, StatusNearing, StatusUnknown}); got != StatusNearing {
		t.Fatalf("got %q, want nearing", got)
	}
	if got := WorstStatus(nil); got != StatusUnknown {
		t.Fatalf("got %q, want unknown", got)
	}
}
