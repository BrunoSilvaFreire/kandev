package usage

// RemainingPct returns the remaining subscription percentage for a usage
// snapshot, using the most-utilized window. It returns known=false when the
// snapshot is nil or has no windows, which callers treat as "unknown" rather
// than as full or zero quota.
func RemainingPct(usage *ProviderUsage) (pct float64, known bool) {
	if usage == nil || len(usage.Windows) == 0 {
		return 0, false
	}
	maxUtilization := usage.Windows[0].UtilizationPct
	for _, window := range usage.Windows[1:] {
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
