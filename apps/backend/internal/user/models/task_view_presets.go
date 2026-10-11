package models

import (
	"strings"
	"unicode/utf8"
)

const (
	// MaxTaskViewGroupLevels bounds one view's nested group keys.
	MaxTaskViewGroupLevels = 4
	// MaxTaskViewPresets bounds one view's named presets.
	MaxTaskViewPresets = 20
	// MaxTaskViewPresetNameRunes bounds one preset's display name.
	MaxTaskViewPresetNameRunes = 80
	// TaskViewPresetVersion is the preset shape this build reads and writes.
	TaskViewPresetVersion = 1

	groupKeyNone = "none"
)

// TaskViewPreset is one named filter and grouping setup for a task view. The
// ID is stable across renames. A preset written by a newer shape version is
// dropped on read instead of being misinterpreted.
type TaskViewPreset struct {
	Version int                `json:"version"`
	ID      string             `json:"id"`
	Name    string             `json:"name"`
	Filters []ViewFilterClause `json:"filters"`
	Groups  []string           `json:"groups"`
}

// normalizeGroupOrder keeps known keys once each, in order. "none" ends the
// list, and an empty result is the explicit ["none"].
func normalizeGroupOrder(keys []string) []string {
	order := make([]string, 0, len(keys))
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		if key == groupKeyNone || len(order) >= MaxTaskViewGroupLevels {
			break
		}
		if _, known := ViewFilterGroupKeys[key]; !known {
			continue
		}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		order = append(order, key)
	}
	if len(order) == 0 {
		return []string{groupKeyNone}
	}
	return order
}

// NormalizeTaskViewGroupOrder keeps known task views with their normalized
// ordered group keys. A view that only has the single-key compatibility value
// in legacy is lifted into a one-key list, so an existing preference loads
// unchanged.
func NormalizeTaskViewGroupOrder(value map[string][]string, legacy map[string]string) map[string][]string {
	normalized := make(map[string][]string)
	for view, group := range NormalizeTaskViewGroups(legacy) {
		normalized[view] = normalizeGroupOrder([]string{group})
	}
	for view, keys := range value {
		if _, ok := TaskViewIDs[view]; !ok {
			continue
		}
		normalized[view] = normalizeGroupOrder(keys)
	}
	return normalized
}

// LegacyTaskViewGroups projects ordered group keys onto the single-key
// compatibility field: the outermost key of each view.
func LegacyTaskViewGroups(order map[string][]string) map[string]string {
	legacy := make(map[string]string, len(order))
	for view, keys := range order {
		if len(keys) > 0 {
			legacy[view] = keys[0]
		}
	}
	return NormalizeTaskViewGroups(legacy)
}

// NormalizeTaskViewPresets keeps known task views and well-formed presets:
// a readable version, a non-empty ID and name, a unique ID, and a name that is
// unique in its view ignoring case and surrounding space. The first of two
// colliding presets wins, order is preserved, and the list is capped.
func NormalizeTaskViewPresets(value map[string][]TaskViewPreset) map[string][]TaskViewPreset {
	normalized := make(map[string][]TaskViewPreset)
	for view, presets := range value {
		if _, ok := TaskViewIDs[view]; !ok {
			continue
		}
		kept := make([]TaskViewPreset, 0, len(presets))
		ids := make(map[string]struct{}, len(presets))
		names := make(map[string]struct{}, len(presets))
		for _, preset := range presets {
			if len(kept) >= MaxTaskViewPresets {
				break
			}
			name := strings.TrimSpace(preset.Name)
			nameKey := strings.ToLower(name)
			_, dupID := ids[preset.ID]
			_, dupName := names[nameKey]
			if preset.Version > TaskViewPresetVersion || preset.ID == "" || name == "" ||
				utf8.RuneCountInString(name) > MaxTaskViewPresetNameRunes || dupID || dupName {
				continue
			}
			ids[preset.ID] = struct{}{}
			names[nameKey] = struct{}{}
			filters := NormalizeTaskViewFilters(map[string][]ViewFilterClause{view: preset.Filters})[view]
			kept = append(kept, TaskViewPreset{
				Version: TaskViewPresetVersion,
				ID:      preset.ID,
				Name:    name,
				Filters: filters,
				Groups:  normalizeGroupOrder(preset.Groups),
			})
		}
		normalized[view] = kept
	}
	return normalized
}

// NormalizeTaskViewActivePreset keeps an active preset ID only while it names
// a preset of that view.
func NormalizeTaskViewActivePreset(active map[string]string, presets map[string][]TaskViewPreset) map[string]string {
	normalized := make(map[string]string)
	for view, id := range active {
		for _, preset := range presets[view] {
			if preset.ID == id && id != "" {
				normalized[view] = id
			}
		}
	}
	return normalized
}
