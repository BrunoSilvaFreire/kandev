package models

import (
	"reflect"
	"strings"
	"testing"
)

func TestNormalizeTaskViewGroupOrder(t *testing.T) {
	got := NormalizeTaskViewGroupOrder(
		map[string][]string{
			"kanban":  {"repositoryGroup", "bogus", "workflow", "repositoryGroup"},
			"threads": {"state"},
		},
		map[string]string{"kanban": "state", "list": "priority"},
	)
	want := map[string][]string{
		// An explicit list wins over the single-key compatibility value.
		"kanban": {"repositoryGroup", "workflow"},
		// A legacy single key is lifted into a one-key list.
		"list": {"priority"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if got := NormalizeTaskViewGroupOrder(map[string][]string{"list": {"state", "none", "workflow"}}, nil); !reflect.DeepEqual(got["list"], []string{"state"}) {
		t.Fatalf("none must end the list, got %v", got["list"])
	}
	if got := NormalizeTaskViewGroupOrder(map[string][]string{"list": {}}, nil); !reflect.DeepEqual(got["list"], []string{"none"}) {
		t.Fatalf("empty list must be explicit none, got %v", got["list"])
	}
	if got := LegacyTaskViewGroups(want); got["kanban"] != "repositoryGroup" || got["list"] != "priority" {
		t.Fatalf("legacy projection must be the outermost key, got %v", got)
	}
}

func TestNormalizeTaskViewPresets(t *testing.T) {
	clause := ViewFilterClause{ID: "c1", Dimension: "state", Op: "in", Value: []byte(`["REVIEW"]`)}
	got := NormalizeTaskViewPresets(map[string][]TaskViewPreset{
		"kanban": {
			{ID: "p1", Name: "  Mine  ", Filters: []ViewFilterClause{clause, {ID: "", Dimension: "x", Op: "is"}}, Groups: []string{"repositoryGroup", "workflow"}},
			{ID: "p2", Name: "mine"},                                       // name collides ignoring case and space
			{ID: "p1", Name: "Other"},                                      // ID collides
			{ID: "p3", Name: " "},                                          // empty name
			{ID: "p4", Name: strings.Repeat("x", 81)},                      // name too long
			{ID: "p5", Name: "Future", Version: TaskViewPresetVersion + 1}, // unreadable shape
			{ID: "p6", Name: "Second"},
		},
		"threads": {{ID: "t1", Name: "Ignored view"}},
	})
	presets := got["kanban"]
	if len(got) != 1 || len(presets) != 2 || presets[0].ID != "p1" || presets[1].ID != "p6" {
		t.Fatalf("got %+v", got)
	}
	if presets[0].Name != "Mine" || presets[0].Version != TaskViewPresetVersion {
		t.Fatalf("name/version not normalized: %+v", presets[0])
	}
	if len(presets[0].Filters) != 1 || !reflect.DeepEqual(presets[0].Groups, []string{"repositoryGroup", "workflow"}) {
		t.Fatalf("filters/groups not preserved: %+v", presets[0])
	}
	if !reflect.DeepEqual(presets[1].Groups, []string{"none"}) {
		t.Fatalf("missing groups must be explicit none: %+v", presets[1])
	}

	many := make([]TaskViewPreset, 0, MaxTaskViewPresets+5)
	for i := 0; i < MaxTaskViewPresets+5; i++ {
		id := strings.Repeat("a", i+1)
		many = append(many, TaskViewPreset{ID: id, Name: id})
	}
	if capped := NormalizeTaskViewPresets(map[string][]TaskViewPreset{"list": many})["list"]; len(capped) != MaxTaskViewPresets {
		t.Fatalf("expected cap %d, got %d", MaxTaskViewPresets, len(capped))
	}

	active := NormalizeTaskViewActivePreset(map[string]string{"kanban": "p6", "list": "gone"}, got)
	if len(active) != 1 || active["kanban"] != "p6" {
		t.Fatalf("active must name an existing preset, got %v", active)
	}
}
