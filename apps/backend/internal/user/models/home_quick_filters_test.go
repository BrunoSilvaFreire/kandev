package models

import "testing"

func TestNormalizeHomeQuickFilters(t *testing.T) {
	t.Run("drops unknown views and empty dimensions", func(t *testing.T) {
		got := NormalizeHomeQuickFilters(map[string][]string{
			"kanban": {"repositoryGroup", "", "state"},
			"bogus":  {"state"},
		})
		want := map[string][]string{"kanban": {"repositoryGroup", "state"}}
		if len(got) != len(want) {
			t.Fatalf("got %v, want %v", got, want)
		}
		if got["kanban"][0] != "repositoryGroup" || got["kanban"][1] != "state" {
			t.Fatalf("got %v, want %v", got, want)
		}
	})

	t.Run("de-duplicates dimensions and preserves order", func(t *testing.T) {
		got := NormalizeHomeQuickFilters(map[string][]string{
			"list": {"repository", "repository", "workflow"},
		})
		if len(got["list"]) != 2 || got["list"][0] != "repository" || got["list"][1] != "workflow" {
			t.Fatalf("got %v", got["list"])
		}
	})

	t.Run("nil yields an empty non-nil map", func(t *testing.T) {
		got := NormalizeHomeQuickFilters(nil)
		if got == nil {
			t.Fatal("expected non-nil map")
		}
		if len(got) != 0 {
			t.Fatalf("expected empty map, got %v", got)
		}
	})
}
