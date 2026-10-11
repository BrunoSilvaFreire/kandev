import type { ViewFilterClause, ViewGroupKey, ViewPreset } from "./types";

/** A view keeps at most this many presets; the backend enforces the same cap. */
export const MAX_VIEW_PRESETS = 20;
export const MAX_VIEW_PRESET_NAME_LENGTH = 80;

/** Why a preset name was refused. Callers map each code to localized copy. */
export type ViewPresetNameError = "empty" | "too_long" | "duplicate" | "limit";

function nameKey(name: string): string {
  return name.trim().toLowerCase();
}

/**
 * Validates a name for a new preset, or for renaming `ignoreId`. Names are
 * unique within a view ignoring case and surrounding space.
 */
export function validateViewPresetName(
  presets: readonly ViewPreset[],
  name: string,
  ignoreId?: string,
): ViewPresetNameError | null {
  const trimmed = name.trim();
  if (trimmed === "") return "empty";
  if ([...trimmed].length > MAX_VIEW_PRESET_NAME_LENGTH) return "too_long";
  const taken = presets.some(
    (preset) => preset.id !== ignoreId && nameKey(preset.name) === nameKey(trimmed),
  );
  if (taken) return "duplicate";
  if (ignoreId === undefined && presets.length >= MAX_VIEW_PRESETS) return "limit";
  return null;
}

/** True when the live filters or grouping differ from the preset. */
export function isViewPresetDirty(
  preset: ViewPreset | undefined,
  filters: readonly ViewFilterClause[],
  groups: readonly ViewGroupKey[],
): boolean {
  if (!preset) return false;
  return (
    JSON.stringify(preset.filters) !== JSON.stringify(filters) ||
    JSON.stringify(preset.groups) !== JSON.stringify(groups)
  );
}

/** Appends a preset capturing the live filters and grouping. */
export function addViewPreset(presets: readonly ViewPreset[], preset: ViewPreset): ViewPreset[] {
  return [...presets, { ...preset, name: preset.name.trim() }];
}

/** Overwrites one preset's filters and grouping, keeping its identity and name. */
export function updateViewPreset(
  presets: readonly ViewPreset[],
  id: string,
  filters: readonly ViewFilterClause[],
  groups: readonly ViewGroupKey[],
): ViewPreset[] {
  return presets.map((preset) =>
    preset.id === id ? { ...preset, filters: [...filters], groups: [...groups] } : preset,
  );
}

/** Renames one preset in place; its identity and position are unchanged. */
export function renameViewPreset(
  presets: readonly ViewPreset[],
  id: string,
  name: string,
): ViewPreset[] {
  return presets.map((preset) => (preset.id === id ? { ...preset, name: name.trim() } : preset));
}

export function removeViewPreset(presets: readonly ViewPreset[], id: string): ViewPreset[] {
  return presets.filter((preset) => preset.id !== id);
}
