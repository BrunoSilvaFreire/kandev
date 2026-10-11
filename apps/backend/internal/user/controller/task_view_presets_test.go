package controller

import (
	"context"
	"reflect"
	"testing"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/user/dto"
	"github.com/kandev/kandev/internal/user/models"
	"github.com/kandev/kandev/internal/user/service"
)

// The settings PATCH carries the task view filters, ordered group keys, named
// presets, and active preset through to storage, and keeps the single-key
// compatibility field in step with the outermost group.
func TestUpdateUserSettingsPersistsTaskViewFiltersGroupsAndPresets(t *testing.T) {
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("logger.NewFromZap: %v", err)
	}
	clause := models.ViewFilterClause{ID: "c1", Dimension: "state", Op: "in", Value: []byte(`["REVIEW"]`)}
	repo := &settingsRepository{settings: &models.UserSettings{
		// A stored single-key preference from before ordered groups existed.
		TaskViewGroups: map[string]string{"list": "priority"},
	}}
	controller := NewController(service.NewService(repo, nil, log))

	filters := map[string][]models.ViewFilterClause{"kanban": {clause}}
	order := map[string][]string{"kanban": {"repositoryGroup", "workflow"}}
	presets := map[string][]models.TaskViewPreset{"kanban": {
		{ID: "p1", Name: "Mine", Filters: []models.ViewFilterClause{clause}, Groups: []string{"repositoryGroup", "workflow"}},
	}}
	active := map[string]string{"kanban": "p1"}
	response, err := controller.UpdateUserSettings(context.Background(), dto.UpdateUserSettingsRequest{
		TaskViewFilters:      &filters,
		TaskViewGroupOrder:   &order,
		TaskViewPresets:      &presets,
		TaskViewActivePreset: &active,
	})
	if err != nil {
		t.Fatalf("UpdateUserSettings: %v", err)
	}

	got := response.Settings
	if len(got.TaskViewFilters["kanban"]) != 1 {
		t.Fatalf("filters were not persisted: %+v", got.TaskViewFilters)
	}
	if !reflect.DeepEqual(got.TaskViewGroupOrder["kanban"], []string{"repositoryGroup", "workflow"}) {
		t.Fatalf("group order = %v", got.TaskViewGroupOrder)
	}
	if got.TaskViewGroups["kanban"] != "repositoryGroup" {
		t.Fatalf("compatibility group must be the outermost key, got %v", got.TaskViewGroups)
	}
	if len(got.TaskViewPresets["kanban"]) != 1 || got.TaskViewActivePreset["kanban"] != "p1" {
		t.Fatalf("presets/active = %+v / %v", got.TaskViewPresets, got.TaskViewActivePreset)
	}
	if !reflect.DeepEqual(repo.settings.TaskViewGroupOrder["kanban"], order["kanban"]) {
		t.Fatalf("stored group order = %v", repo.settings.TaskViewGroupOrder)
	}

	// Deleting the active preset clears the active ID and keeps the live
	// filters and grouping.
	none := map[string][]models.TaskViewPreset{"kanban": {}}
	response, err = controller.UpdateUserSettings(context.Background(), dto.UpdateUserSettingsRequest{
		TaskViewPresets: &none,
	})
	if err != nil {
		t.Fatalf("delete preset: %v", err)
	}
	got = response.Settings
	if _, stillActive := got.TaskViewActivePreset["kanban"]; stillActive {
		t.Fatalf("active preset must clear with its preset, got %v", got.TaskViewActivePreset)
	}
	if len(got.TaskViewFilters["kanban"]) != 1 || len(got.TaskViewGroupOrder["kanban"]) != 2 {
		t.Fatalf("deleting a preset must keep live filters and grouping: %+v", got)
	}

	// A client that only knows the single key still drives grouping.
	single := map[string]string{"kanban": "state"}
	response, err = controller.UpdateUserSettings(context.Background(), dto.UpdateUserSettingsRequest{
		TaskViewGroups: &single,
	})
	if err != nil {
		t.Fatalf("single-key write: %v", err)
	}
	if !reflect.DeepEqual(response.Settings.TaskViewGroupOrder["kanban"], []string{"state"}) {
		t.Fatalf("single-key write must become a one-key list, got %v", response.Settings.TaskViewGroupOrder)
	}
}
