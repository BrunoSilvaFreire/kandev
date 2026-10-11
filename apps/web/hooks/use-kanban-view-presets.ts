"use client";

import { useMemo } from "react";
import { generateUUID } from "@/lib/uuid";
import {
  addViewPreset,
  isViewPresetDirty,
  removeViewPreset,
  renameViewPreset,
  updateViewPreset,
  validateViewPresetName,
  type ViewPresetNameError,
} from "@/lib/view-model/presets";
import type { ViewFilterClause, ViewGroupKey, ViewPreset } from "@/lib/view-model/types";

type PresetSettings = {
  taskViewFilters?: Record<string, ViewFilterClause[]>;
  taskViewGroups?: Record<string, string>;
  taskViewGroupOrder?: Record<string, string[]>;
  taskViewPresets?: Record<string, ViewPreset[]>;
  taskViewActivePreset?: Record<string, string>;
};

type UseKanbanViewPresetsInput = {
  taskView: "kanban" | "list";
  filters: readonly ViewFilterClause[];
  groups: readonly ViewGroupKey[];
  userSettings: PresetSettings;
  commit: (patch: PresetSettings) => void;
};

export type KanbanViewPresets = {
  items: ViewPreset[];
  active: ViewPreset | undefined;
  /** True when the live filters or grouping differ from the active preset. */
  dirty: boolean;
  validateName: (name: string, ignoreId?: string) => ViewPresetNameError | null;
  apply: (id: string) => void;
  /** Saves the live filters and grouping as a new preset and activates it. */
  create: (name: string) => ViewPresetNameError | null;
  /** Overwrites the active preset with the live filters and grouping. */
  saveActive: () => void;
  rename: (id: string, name: string) => ViewPresetNameError | null;
  remove: (id: string) => void;
};

const EMPTY_PRESETS: ViewPreset[] = [];

/**
 * Named filter and grouping presets for the Kanban or List view. Every write
 * is a whole-list replacement through the shared user-settings commit;
 * applying takes effect immediately and saving is always explicit.
 */
export function useKanbanViewPresets({
  taskView,
  filters,
  groups,
  userSettings,
  commit,
}: UseKanbanViewPresetsInput): KanbanViewPresets {
  const items = userSettings.taskViewPresets?.[taskView] ?? EMPTY_PRESETS;
  const activeId = userSettings.taskViewActivePreset?.[taskView];
  const active = useMemo(() => items.find((preset) => preset.id === activeId), [items, activeId]);
  const dirty = useMemo(
    () => isViewPresetDirty(active, filters, groups),
    [active, filters, groups],
  );

  const withPresets = (next: ViewPreset[]): PresetSettings => ({
    taskViewPresets: { ...(userSettings.taskViewPresets ?? {}), [taskView]: next },
  });
  const withActive = (id: string | null): PresetSettings => {
    const next = { ...(userSettings.taskViewActivePreset ?? {}) };
    if (id) next[taskView] = id;
    else delete next[taskView];
    return { taskViewActivePreset: next };
  };

  return {
    items,
    active,
    dirty,
    validateName: (name, ignoreId) => validateViewPresetName(items, name, ignoreId),
    apply: (id) => {
      const preset = items.find((candidate) => candidate.id === id);
      if (!preset) return;
      commit({
        ...withActive(id),
        taskViewFilters: { ...(userSettings.taskViewFilters ?? {}), [taskView]: preset.filters },
        taskViewGroupOrder: {
          ...(userSettings.taskViewGroupOrder ?? {}),
          [taskView]: preset.groups,
        },
        taskViewGroups: { ...(userSettings.taskViewGroups ?? {}), [taskView]: preset.groups[0] },
      });
    },
    create: (name) => {
      const error = validateViewPresetName(items, name);
      if (error) return error;
      const preset: ViewPreset = {
        id: generateUUID(),
        name,
        filters: [...filters],
        groups: [...groups],
      };
      commit({ ...withPresets(addViewPreset(items, preset)), ...withActive(preset.id) });
      return null;
    },
    saveActive: () => {
      if (!active) return;
      commit(withPresets(updateViewPreset(items, active.id, filters, groups)));
    },
    rename: (id, name) => {
      const error = validateViewPresetName(items, name, id);
      if (error) return error;
      commit(withPresets(renameViewPreset(items, id, name)));
      return null;
    },
    remove: (id) => {
      commit({
        ...withPresets(removeViewPreset(items, id)),
        ...(id === activeId ? withActive(null) : {}),
      });
    },
  };
}
