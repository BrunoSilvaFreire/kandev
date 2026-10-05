package models

import "encoding/json"

// TaskViewIDs are the Home views that persist their active filter/group state
// in user settings rather than in a dedicated per-view record (Threads and the
// sidebar own their own view records).
var TaskViewIDs = map[string]struct{}{
	"kanban": {},
	"list":   {},
}

// ViewFilterGroupKeys are the group keys the shared frontend view model renders.
var ViewFilterGroupKeys = map[string]struct{}{
	"none":            {},
	"repository":      {},
	"repositoryGroup": {},
	"workflow":        {},
	"workflowStep":    {},
	"executorType":    {},
	"state":           {},
	"priority":        {},
}

// ViewFilterOps are the filter operators the shared frontend view model accepts.
var ViewFilterOps = map[string]struct{}{
	"is":          {},
	"is_not":      {},
	"in":          {},
	"not_in":      {},
	"matches":     {},
	"not_matches": {},
}

const (
	// MaxTaskViewFilterClauses bounds one view's clause count.
	MaxTaskViewFilterClauses = 20
	// MaxTaskViewFilterValueBytes bounds one clause value's serialized size.
	MaxTaskViewFilterValueBytes = 4096
)

// ViewFilterClause mirrors the shared frontend `ViewFilterClause` shape. The
// value is kept as raw JSON because it may be a string, a string array, or a
// boolean; the frontend owns the per-dimension interpretation.
type ViewFilterClause struct {
	ID        string          `json:"id"`
	Dimension string          `json:"dimension"`
	Op        string          `json:"op"`
	Value     json.RawMessage `json:"value"`
}

// NormalizeTaskViewFilters keeps only known task views and drops clauses with an
// empty id or dimension, an unknown operator, an oversized value, or a duplicate
// id. A nil input yields an empty (non-nil) map so the wire payload always
// carries an object.
func NormalizeTaskViewFilters(value map[string][]ViewFilterClause) map[string][]ViewFilterClause {
	normalized := make(map[string][]ViewFilterClause)
	for view, clauses := range value {
		if _, ok := TaskViewIDs[view]; !ok {
			continue
		}
		seen := make(map[string]struct{}, len(clauses))
		kept := make([]ViewFilterClause, 0, len(clauses))
		for _, clause := range clauses {
			if len(kept) >= MaxTaskViewFilterClauses {
				break
			}
			if clause.ID == "" || clause.Dimension == "" {
				continue
			}
			if _, ok := ViewFilterOps[clause.Op]; !ok {
				continue
			}
			if len(clause.Value) > MaxTaskViewFilterValueBytes {
				continue
			}
			if _, ok := seen[clause.ID]; ok {
				continue
			}
			seen[clause.ID] = struct{}{}
			kept = append(kept, clause)
		}
		normalized[view] = kept
	}
	return normalized
}

// NormalizeTaskViewGroups keeps only known task views and known group keys. A
// nil input yields an empty (non-nil) map.
func NormalizeTaskViewGroups(value map[string]string) map[string]string {
	normalized := make(map[string]string)
	for view, group := range value {
		if _, ok := TaskViewIDs[view]; !ok {
			continue
		}
		if _, ok := ViewFilterGroupKeys[group]; !ok {
			continue
		}
		normalized[view] = group
	}
	return normalized
}
