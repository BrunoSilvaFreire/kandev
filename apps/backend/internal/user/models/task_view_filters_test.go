package models

import (
	"encoding/json"
	"testing"
)

func TestNormalizeTaskViewFilters(t *testing.T) {
	clause := func(id, dimension, op string, value any) ViewFilterClause {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("marshal value: %v", err)
		}
		return ViewFilterClause{ID: id, Dimension: dimension, Op: op, Value: raw}
	}

	t.Run("drops unknown views and malformed clauses", func(t *testing.T) {
		got := NormalizeTaskViewFilters(map[string][]ViewFilterClause{
			"kanban": {
				clause("a", "repository", "in", []string{"r1"}),
				clause("", "repository", "in", "r1"),
				{ID: "c", Dimension: "", Op: "is", Value: json.RawMessage(`true`)},
				{ID: "d", Dimension: "state", Op: "bogus", Value: json.RawMessage(`"x"`)},
			},
			"threads": {clause("e", "repository", "in", "r1")},
		})
		if len(got) != 1 {
			t.Fatalf("expected only kanban, got %v", got)
		}
		if len(got["kanban"]) != 1 || got["kanban"][0].ID != "a" {
			t.Fatalf("got %v", got["kanban"])
		}
	})

	t.Run("de-duplicates by id and preserves order", func(t *testing.T) {
		got := NormalizeTaskViewFilters(map[string][]ViewFilterClause{
			"list": {
				clause("a", "state", "in", []string{"REVIEW"}),
				clause("a", "state", "is", "TODO"),
				clause("b", "priority", "is", "high"),
			},
		})
		if len(got["list"]) != 2 || got["list"][0].ID != "a" || got["list"][1].ID != "b" {
			t.Fatalf("got %v", got["list"])
		}
	})

	t.Run("caps clause count", func(t *testing.T) {
		clauses := make([]ViewFilterClause, 0, MaxTaskViewFilterClauses+5)
		for i := 0; i < MaxTaskViewFilterClauses+5; i++ {
			clauses = append(clauses, clause(string(rune('a'+i%26))+string(rune('0'+i/26)), "state", "is", "TODO"))
		}
		got := NormalizeTaskViewFilters(map[string][]ViewFilterClause{"kanban": clauses})
		if len(got["kanban"]) != MaxTaskViewFilterClauses {
			t.Fatalf("expected %d clauses, got %d", MaxTaskViewFilterClauses, len(got["kanban"]))
		}
	})

	t.Run("nil yields an empty non-nil map", func(t *testing.T) {
		got := NormalizeTaskViewFilters(nil)
		if got == nil || len(got) != 0 {
			t.Fatalf("expected empty non-nil map, got %v", got)
		}
	})
}

func TestNormalizeTaskViewGroups(t *testing.T) {
	got := NormalizeTaskViewGroups(map[string]string{
		"kanban":  "repositoryGroup",
		"list":    "bogus",
		"threads": "state",
	})
	if len(got) != 1 || got["kanban"] != "repositoryGroup" {
		t.Fatalf("got %v", got)
	}
	if got := NormalizeTaskViewGroups(nil); got == nil || len(got) != 0 {
		t.Fatalf("expected empty non-nil map, got %v", got)
	}
}
