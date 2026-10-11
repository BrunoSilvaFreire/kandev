import { describe, expect, it } from "vitest";
import {
  MAX_VIEW_PRESETS,
  addViewPreset,
  isViewPresetDirty,
  removeViewPreset,
  renameViewPreset,
  updateViewPreset,
  validateViewPresetName,
} from "./presets";
import type { ViewFilterClause, ViewPreset } from "./types";

const clause: ViewFilterClause = { id: "c1", dimension: "state", op: "in", value: ["REVIEW"] };
const mine: ViewPreset = {
  id: "p1",
  name: "Mine",
  filters: [clause],
  groups: ["repositoryGroup", "workflow"],
};
const other: ViewPreset = { id: "p2", name: "Other", filters: [], groups: ["none"] };

describe("validateViewPresetName", () => {
  it("rejects an empty or over-long name", () => {
    expect(validateViewPresetName([], "   ")).toBe("empty");
    expect(validateViewPresetName([], "x".repeat(81))).toBe("too_long");
  });

  it("rejects a name that collides ignoring case and surrounding space", () => {
    expect(validateViewPresetName([mine], "  mINE ")).toBe("duplicate");
    expect(validateViewPresetName([mine], "Mine 2")).toBeNull();
  });

  it("lets a preset keep its own name when renamed", () => {
    expect(validateViewPresetName([mine, other], "mine", "p1")).toBeNull();
    expect(validateViewPresetName([mine, other], "other", "p1")).toBe("duplicate");
  });

  it("refuses a new preset past the cap but still allows renames", () => {
    const full = Array.from({ length: MAX_VIEW_PRESETS }, (_, index) => ({
      ...other,
      id: `p${index}`,
      name: `Preset ${index}`,
    }));
    expect(validateViewPresetName(full, "One more")).toBe("limit");
    expect(validateViewPresetName(full, "Renamed", "p0")).toBeNull();
  });
});

describe("preset list operations", () => {
  it("keeps identity and order through rename and update", () => {
    const renamed = renameViewPreset([mine, other], "p1", "  Renamed ");
    expect(renamed.map((preset) => [preset.id, preset.name])).toEqual([
      ["p1", "Renamed"],
      ["p2", "Other"],
    ]);
    const updated = updateViewPreset(renamed, "p2", [clause], ["state"]);
    expect(updated[1]).toEqual({ id: "p2", name: "Other", filters: [clause], groups: ["state"] });
    expect(updated[0]).toEqual(renamed[0]);
  });

  it("appends a trimmed preset and removes by identity", () => {
    const added = addViewPreset([mine], { ...other, name: " Other " });
    expect(added.map((preset) => preset.name)).toEqual(["Mine", "Other"]);
    expect(removeViewPreset(added, "p1").map((preset) => preset.id)).toEqual(["p2"]);
  });
});

describe("isViewPresetDirty", () => {
  it("is clean when live state matches and when no preset is active", () => {
    expect(isViewPresetDirty(mine, [clause], ["repositoryGroup", "workflow"])).toBe(false);
    expect(isViewPresetDirty(undefined, [clause], ["state"])).toBe(false);
  });

  it("is dirty when filters, grouping, or group order differ", () => {
    expect(isViewPresetDirty(mine, [], ["repositoryGroup", "workflow"])).toBe(true);
    expect(isViewPresetDirty(mine, [clause], ["repositoryGroup"])).toBe(true);
    expect(isViewPresetDirty(mine, [clause], ["workflow", "repositoryGroup"])).toBe(true);
  });
});
