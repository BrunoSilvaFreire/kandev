package store

import (
	"context"
	"reflect"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/user/models"
)

// Ordered group keys, presets, and the active preset survive a store
// round-trip, and a row that only has the single-key field loads as a one-key
// list with its filters intact.
func TestSQLiteRepositoryTaskViewPresetsRoundTrip(t *testing.T) {
	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	repo, err := newSQLiteRepositoryWithDB(conn, conn)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	ctx := context.Background()
	settings, err := repo.GetUserSettings(ctx, DefaultUserID)
	if err != nil {
		t.Fatalf("get defaults: %v", err)
	}
	clause := models.ViewFilterClause{ID: "c1", Dimension: "state", Op: "in", Value: []byte(`["REVIEW"]`)}
	settings.TaskViewFilters = map[string][]models.ViewFilterClause{"kanban": {clause}, "list": {clause}}
	settings.TaskViewGroups = map[string]string{"kanban": "repositoryGroup", "list": "priority"}
	settings.TaskViewGroupOrder = map[string][]string{"kanban": {"repositoryGroup", "workflow"}}
	settings.TaskViewPresets = map[string][]models.TaskViewPreset{"kanban": {
		{ID: "p1", Name: "Mine", Filters: []models.ViewFilterClause{clause}, Groups: []string{"repositoryGroup", "workflow"}},
		{ID: "p2", Name: "Second"},
	}}
	settings.TaskViewActivePreset = map[string]string{"kanban": "p2"}
	upsertUserSettingsForTest(t, repo, ctx, settings)

	got, err := repo.GetUserSettings(ctx, DefaultUserID)
	if err != nil {
		t.Fatalf("get saved settings: %v", err)
	}
	if !reflect.DeepEqual(got.TaskViewGroupOrder["kanban"], []string{"repositoryGroup", "workflow"}) {
		t.Fatalf("kanban order = %v", got.TaskViewGroupOrder)
	}
	if !reflect.DeepEqual(got.TaskViewGroupOrder["list"], []string{"priority"}) {
		t.Fatalf("a single-key preference must load as a one-key list, got %v", got.TaskViewGroupOrder)
	}
	if len(got.TaskViewFilters["list"]) != 1 {
		t.Fatalf("filters must survive the lift, got %+v", got.TaskViewFilters)
	}
	presets := got.TaskViewPresets["kanban"]
	if len(presets) != 2 || presets[0].ID != "p1" || presets[1].ID != "p2" {
		t.Fatalf("preset order/identity lost: %+v", presets)
	}
	if got.TaskViewActivePreset["kanban"] != "p2" {
		t.Fatalf("active preset = %v", got.TaskViewActivePreset)
	}
}
