import { describe, expect, it } from "vitest";
import { applyViewFilters } from "./filters";
import { applyViewGroup } from "./groups";
import {
  repositoryGroupKeyAndLabel,
  taskMatchesRepositoryGroup,
  UNGROUPED_REPOSITORY_GROUP_KEY,
} from "./repository-group";
import { resolveQuickFilterDimensions } from "./quick-filters";
import { getViewDimensionMeta } from "./dimensions";
import { SUPPORTED_DIMENSIONS, SUPPORTED_GROUPS, viewSupportsGroup } from "./views";
import type { ViewFilterClause, ViewValueAccessor } from "./index";

type Item = { id: string; repo: string; priority: string; title: string };

const ITEMS: Item[] = [
  { id: "a", repo: "front", priority: "high", title: "Add a button" },
  { id: "b", repo: "back", priority: "low", title: "Fix API" },
  { id: "c", repo: "front", priority: "high", title: "Add a modal" },
];

const accessor: ViewValueAccessor<Item, "repo" | "priority" | "title"> = (item, dimension) => {
  if (dimension === "repo") return item.repo;
  if (dimension === "priority") return item.priority;
  return item.title;
};

function clause(partial: Partial<ViewFilterClause<"repo" | "priority" | "title">>) {
  return { id: partial.id ?? "c1", ...partial } as ViewFilterClause<"repo" | "priority" | "title">;
}

describe("applyViewFilters", () => {
  it("keeps every item with no clauses", () => {
    expect(applyViewFilters(ITEMS, [], accessor)).toBe(ITEMS);
  });

  it("ANDs clauses and ORs multiple values in one clause", () => {
    const filtered = applyViewFilters(
      ITEMS,
      [
        clause({ dimension: "repo", op: "in", value: ["front"] }),
        clause({ dimension: "priority", op: "in", value: ["high"] }),
      ],
      accessor,
    );
    expect(filtered.map((item) => item.id)).toEqual(["a", "c"]);
  });

  it("supports text matching", () => {
    const filtered = applyViewFilters(
      ITEMS,
      [clause({ dimension: "title", op: "matches", value: "add" })],
      accessor,
    );
    expect(filtered.map((item) => item.id)).toEqual(["a", "c"]);
  });
});

describe("applyViewGroup", () => {
  it("returns one pass-through group for none", () => {
    const grouped = applyViewGroup(ITEMS, "none", () => ({ key: "unused", label: "unused" }));
    expect(grouped.groups).toHaveLength(1);
    expect(grouped.groups[0].items).toBe(ITEMS);
  });

  it("groups by the extractor and preserves first-seen order", () => {
    const grouped = applyViewGroup(ITEMS, "repository", (item) => ({
      key: item.repo,
      label: item.repo,
    }));
    expect(grouped.groups.map((group) => group.key)).toEqual(["front", "back"]);
    expect(grouped.groups[0].items.map((item) => item.id)).toEqual(["a", "c"]);
  });
});

describe("view capabilities", () => {
  it("provides shared metadata for every supported dimension", () => {
    for (const view of ["kanban", "list", "threads", "sidebar"] as const) {
      for (const dimension of SUPPORTED_DIMENSIONS[view]) {
        expect(getViewDimensionMeta(dimension).labelKey).toBeTruthy();
      }
    }
  });
  it("declares at least one dimension and group per view", () => {
    for (const view of ["kanban", "list", "threads", "sidebar"] as const) {
      expect(SUPPORTED_DIMENSIONS[view].length).toBeGreaterThan(0);
      expect(SUPPORTED_GROUPS[view].length).toBeGreaterThan(0);
    }
  });

  it("lets every view group by repository", () => {
    for (const view of ["kanban", "list", "threads", "sidebar"] as const) {
      expect(viewSupportsGroup(view, "repository")).toBe(true);
    }
  });

  it("offers the repository-group dimension and group in every Home view", () => {
    for (const view of ["kanban", "list", "threads", "sidebar"] as const) {
      expect(SUPPORTED_DIMENSIONS[view]).toContain("repositoryGroup");
      expect(SUPPORTED_GROUPS[view]).toContain("repositoryGroup");
    }
  });
});

describe("repository group semantics", () => {
  const groups = [
    { id: "cityscape", name: "Cityscape", repositoryIds: ["web", "api"] },
    { id: "minecraft", name: "Minecraft", repositoryIds: ["mod", "api"] },
  ];

  it("matches a task when any of its repositories is a member", () => {
    expect(taskMatchesRepositoryGroup(["api", "other"], groups[0])).toBe(true);
    expect(taskMatchesRepositoryGroup(["other"], groups[0])).toBe(false);
    expect(taskMatchesRepositoryGroup([], groups[0])).toBe(false);
  });

  it("places a task under the first group in set order that holds its first repository", () => {
    // "api" is in both; Cityscape comes first in set order.
    expect(repositoryGroupKeyAndLabel(["api", "web"], groups, "Ungrouped")).toEqual({
      key: "cityscape",
      label: "Cityscape",
    });
  });

  it("puts a task with no matching repository under Ungrouped", () => {
    expect(repositoryGroupKeyAndLabel(["other"], groups, "Ungrouped")).toEqual({
      key: UNGROUPED_REPOSITORY_GROUP_KEY,
      label: "Ungrouped",
    });
    expect(repositoryGroupKeyAndLabel([], groups, "Ungrouped").key).toBe(
      UNGROUPED_REPOSITORY_GROUP_KEY,
    );
  });
});

describe("resolveQuickFilterDimensions", () => {
  it("falls back to the defaults, filtered to the view's supported dimensions", () => {
    expect(resolveQuickFilterDimensions("sidebar", undefined)).toEqual([
      "repositoryGroup",
      "repository",
      "workflow",
      "state",
    ]);
  });

  it("keeps the configured list and drops unsupported dimensions", () => {
    expect(
      resolveQuickFilterDimensions("kanban", ["repository", "threadStatus", "priority"]),
    ).toEqual(["repository", "priority"]);
  });

  it("uses defaults when the configured list is empty", () => {
    expect(resolveQuickFilterDimensions("list", [])).toContain("repositoryGroup");
  });
});

describe("repositoryGroup across every Home view (G5)", () => {
  const views = ["sidebar", "threads", "kanban", "list"] as const;

  it("every view advertises the repositoryGroup filter dimension", () => {
    for (const view of views) {
      expect(SUPPORTED_DIMENSIONS[view]).toContain("repositoryGroup");
    }
  });

  it("every view can group by repositoryGroup", () => {
    for (const view of views) {
      expect(viewSupportsGroup(view, "repositoryGroup")).toBe(true);
    }
  });

  it("resolves repositoryGroup as a default quick filter for every view", () => {
    for (const view of views) {
      expect(resolveQuickFilterDimensions(view, undefined)).toContain("repositoryGroup");
    }
  });
});
