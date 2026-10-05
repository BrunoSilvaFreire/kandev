package usage

import "strings"

// RemainingPct returns the remaining subscription percentage for a usage
// snapshot, using the most-utilized applicable window. It returns known=false
// when the snapshot is nil or has no applicable window, which callers treat as
// "unknown" rather than as full or zero quota.
//
// A window with an empty Model applies to every profile. A window with a
// non-empty Model applies only to a profile whose model matches it (equal, or a
// longer variant of the window's model id). An empty model argument considers
// every window, which preserves the behavior providers without per-model
// buckets relied on.
func RemainingPct(usage *ProviderUsage, model string) (pct float64, known bool) {
	if usage == nil {
		return 0, false
	}
	applicable := make([]UtilizationWindow, 0, len(usage.Windows))
	for _, window := range usage.Windows {
		if modelApplies(model, window.Model) {
			applicable = append(applicable, window)
		}
	}
	if len(applicable) == 0 {
		return 0, false
	}
	maxUtilization := applicable[0].UtilizationPct
	for _, window := range applicable[1:] {
		if window.UtilizationPct > maxUtilization {
			maxUtilization = window.UtilizationPct
		}
	}
	remaining := 100 - maxUtilization
	if remaining < 0 {
		remaining = 0
	}
	if remaining > 100 {
		remaining = 100
	}
	return remaining, true
}

// modelApplies reports whether a window scoped to windowModel applies to a
// profile running model. An empty windowModel is unscoped and always applies;
// an empty model argument considers every window.
func modelApplies(model, windowModel string) bool {
	if windowModel == "" || model == "" {
		return true
	}
	return windowModel == model || strings.HasPrefix(model, windowModel)
}
