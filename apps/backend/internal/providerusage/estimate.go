package providerusage

import (
	"sort"
	"time"
)

// Account statuses.
const (
	StatusExhausted   = "exhausted"
	StatusNearing     = "nearing"
	StatusStale       = "stale"
	StatusHealthy     = "healthy"
	StatusUnknown     = "unknown"
	StatusUnavailable = "unavailable"
)

// Estimate trend values.
const (
	TrendRising  = "rising"
	TrendFlat    = "flat"
	TrendFalling = "falling"
)

// Estimate confidence values.
const (
	ConfidenceNone   = "none"
	ConfidenceLow    = "low"
	ConfidenceMedium = "medium"
)

const (
	nearingThresholdPct = 80.0
	exhaustedPct        = 100.0
	staleAfter          = 15 * time.Minute
	// limitHitTTL bounds how long a limit hit with no reset hint stays
	// unexpired. A transient rate-limit failure must not mark an account
	// exhausted for the whole chart range.
	limitHitTTL         = time.Hour
	trendSlopePerHour   = 0.5
	anomalyMultiplier   = 2.0
	minEstimatePoints   = 2
	minConfidencePoints = 6
	minEstimateSpan     = 15 * time.Minute
	minConfidenceSpan   = time.Hour
)

// WindowUtil is one window reading for status and estimation.
type WindowUtil struct {
	Label          string
	UtilizationPct float64
	ResetAt        time.Time
	Source         string
}

// Point is one utilization reading on a series. Kind is measured, estimated,
// or limit_hit.
type Point struct {
	UtilPct float64
	At      time.Time
	ResetAt time.Time
	Kind    string
}

// Estimate is the projection for one window. It is always labelled estimated.
type Estimate struct {
	BurnRatePctPerHour    float64   `json:"burn_rate_pct_per_hour"`
	ProjectedExhaustionAt time.Time `json:"projected_exhaustion_at,omitempty"`
	ExhaustsBeforeReset   bool      `json:"exhausts_before_reset"`
	Trend                 string    `json:"trend"`
	Anomaly               bool      `json:"anomaly"`
	Confidence            string    `json:"confidence"`
	Source                string    `json:"source"`

	ResetAt time.Time `json:"reset_at,omitempty"`
}

// StatusInput carries everything AccountStatus needs beyond the windows.
type StatusInput struct {
	Windows         []WindowUtil
	HasLimitHit     bool
	LiveFetchFailed bool
	HasSource       bool
	LastObservedAt  time.Time
	Now             time.Time
	// HasLiveAccountState is true when the provider reported a positive
	// balance or an active subscription with no utilization windows. It keeps
	// the account healthy rather than unknown.
	HasLiveAccountState bool
}

// AccountStatus returns one account's status. Precedence follows the
// requirements: exhausted, nearing, stale, then healthy, unavailable, unknown.
func AccountStatus(in StatusInput) string {
	maxPct, has := maxUtilization(in.Windows)
	if in.HasLimitHit || (has && maxPct >= exhaustedPct) {
		return StatusExhausted
	}
	if has && maxPct >= nearingThresholdPct {
		return StatusNearing
	}
	if in.LiveFetchFailed && !in.LastObservedAt.IsZero() &&
		in.Now.Sub(in.LastObservedAt) > staleAfter {
		return StatusStale
	}
	if has || in.HasLiveAccountState {
		return StatusHealthy
	}
	if in.LiveFetchFailed {
		return StatusUnavailable
	}
	if in.HasSource {
		return StatusStale
	}
	return StatusUnknown
}

// StatusRank orders statuses from worst to best for provider roll-up.
func StatusRank(status string) int {
	switch status {
	case StatusExhausted:
		return 0
	case StatusNearing:
		return 1
	case StatusStale:
		return 2
	case StatusUnavailable:
		return 3
	case StatusUnknown:
		return 4
	case StatusHealthy:
		return 5
	default:
		return 6
	}
}

// WorstStatus returns the worst status in the list (zero-length is unknown).
func WorstStatus(statuses []string) string {
	if len(statuses) == 0 {
		return StatusUnknown
	}
	worst := statuses[0]
	for _, status := range statuses[1:] {
		if StatusRank(status) < StatusRank(worst) {
			worst = status
		}
	}
	return worst
}

// EstimateWindow projects one window from its measured points. It never
// returns nil; confidence none means there is not enough data to project.
func EstimateWindow(points []Point, now time.Time) Estimate {
	est := Estimate{Trend: TrendFlat, Confidence: ConfidenceNone, Source: "estimated"}
	window := currentWindow(points)
	if len(window) < minEstimatePoints {
		return est
	}
	span := window[len(window)-1].At.Sub(window[0].At)
	est.Confidence = confidenceFor(len(window), span)
	if est.Confidence == ConfidenceNone {
		return est
	}
	slope := slopePerHour(window)
	est.BurnRatePctPerHour = slope
	est.Trend = trendFor(slope)
	est.Anomaly = hasAnomaly(window, slope)
	last := window[len(window)-1]
	est.ResetAt = last.ResetAt
	if slope > 0 {
		hoursToFull := (exhaustedPct - last.UtilPct) / slope
		if hoursToFull < 0 {
			hoursToFull = 0
		}
		est.ProjectedExhaustionAt = last.At.Add(time.Duration(hoursToFull * float64(time.Hour)))
		if !last.ResetAt.IsZero() {
			est.ExhaustsBeforeReset = est.ProjectedExhaustionAt.Before(last.ResetAt)
		}
	}
	return est
}

// currentWindow keeps points in the current rate-limit window: those sharing
// the newest reset time, or, when no reset is known, those since the last
// utilization drop.
func currentWindow(points []Point) []Point {
	filtered := make([]Point, 0, len(points))
	for _, p := range points {
		if p.Kind == KindLimitHit {
			continue
		}
		filtered = append(filtered, p)
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].At.Before(filtered[j].At) })

	var newestReset time.Time
	for _, p := range filtered {
		if p.ResetAt.After(newestReset) {
			newestReset = p.ResetAt
		}
	}
	if !newestReset.IsZero() {
		window := filtered[:0:0]
		for _, p := range filtered {
			if p.ResetAt.Equal(newestReset) {
				window = append(window, p)
			}
		}
		return window
	}
	start := 0
	for i := 1; i < len(filtered); i++ {
		if filtered[i].UtilPct < filtered[i-1].UtilPct {
			start = i
		}
	}
	return filtered[start:]
}

func confidenceFor(points int, span time.Duration) string {
	if points < minEstimatePoints || span < minEstimateSpan {
		return ConfidenceNone
	}
	if points < minConfidencePoints || span < minConfidenceSpan {
		return ConfidenceLow
	}
	return ConfidenceMedium
}

func trendFor(slope float64) string {
	switch {
	case slope > trendSlopePerHour:
		return TrendRising
	case slope < -trendSlopePerHour:
		return TrendFalling
	default:
		return TrendFlat
	}
}

// slopePerHour is the least-squares slope of utilization against hours.
func slopePerHour(points []Point) float64 {
	if len(points) < 2 {
		return 0
	}
	base := points[0].At
	var sumX, sumY, sumXY, sumXX float64
	for _, p := range points {
		x := p.At.Sub(base).Hours()
		y := p.UtilPct
		sumX += x
		sumY += y
		sumXY += x * y
		sumXX += x * x
	}
	n := float64(len(points))
	denom := n*sumXX - sumX*sumX
	if denom == 0 {
		return 0
	}
	return (n*sumXY - sumX*sumY) / denom
}

// hasAnomaly flags a last-hour slope more than twice the window slope. It
// requires the confidence point floor so a short burst cannot flag one.
func hasAnomaly(points []Point, windowSlope float64) bool {
	if len(points) < minConfidencePoints || windowSlope <= 0 {
		return false
	}
	last := points[len(points)-1]
	cutoff := last.At.Add(-time.Hour)
	recent := make([]Point, 0, len(points))
	for _, p := range points {
		if !p.At.Before(cutoff) {
			recent = append(recent, p)
		}
	}
	if len(recent) < minEstimatePoints {
		return false
	}
	return slopePerHour(recent) > anomalyMultiplier*windowSlope
}

// TokenBucket is one hour of weighted token activity.
type TokenBucket struct {
	At             time.Time
	WeightedTokens int64
}

// Calibration is the percent-per-weighted-token factor from a live snapshot.
// Zero tokens yields zero, which means "no calibration".
func Calibration(currentPct float64, weightedTokens float64) float64 {
	if weightedTokens <= 0 {
		return 0
	}
	return currentPct / weightedTokens
}

// EstimatedSeries turns hourly token buckets into a cumulative estimated
// percentage series. A zero factor yields nil (tokens-only chart).
func EstimatedSeries(buckets []TokenBucket, factor float64) []Point {
	if factor <= 0 || len(buckets) == 0 {
		return nil
	}
	ordered := append([]TokenBucket(nil), buckets...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].At.Before(ordered[j].At) })
	series := make([]Point, 0, len(ordered))
	var cumulative int64
	for _, bucket := range ordered {
		cumulative += bucket.WeightedTokens
		series = append(series, Point{
			UtilPct: factor * float64(cumulative),
			At:      bucket.At,
			Kind:    "estimated",
		})
	}
	return series
}

func maxUtilization(windows []WindowUtil) (float64, bool) {
	if len(windows) == 0 {
		return 0, false
	}
	max := windows[0].UtilizationPct
	for _, w := range windows[1:] {
		if w.UtilizationPct > max {
			max = w.UtilizationPct
		}
	}
	return max, true
}
