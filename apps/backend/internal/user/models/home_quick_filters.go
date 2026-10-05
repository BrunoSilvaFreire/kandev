package models

// HomeViewIDs are the Home views that support Quick Filter configuration.
var HomeViewIDs = map[string]struct{}{
	"kanban":  {},
	"list":    {},
	"threads": {},
	"sidebar": {},
}

// NormalizeHomeQuickFilters keeps only known Home view ids and non-empty,
// de-duplicated dimension values. Unknown views are dropped and the per-view
// dimension order is preserved. A nil input yields an empty (non-nil) map so the
// wire payload always carries an object.
func NormalizeHomeQuickFilters(value map[string][]string) map[string][]string {
	normalized := make(map[string][]string)
	for view, dimensions := range value {
		if _, ok := HomeViewIDs[view]; !ok {
			continue
		}
		seen := make(map[string]struct{}, len(dimensions))
		kept := make([]string, 0, len(dimensions))
		for _, dimension := range dimensions {
			if dimension == "" {
				continue
			}
			if _, ok := seen[dimension]; ok {
				continue
			}
			seen[dimension] = struct{}{}
			kept = append(kept, dimension)
		}
		normalized[view] = kept
	}
	return normalized
}
