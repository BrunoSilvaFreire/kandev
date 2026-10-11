import { describe, expect, it } from "vitest";
import {
  groupKanbanLanes,
  groupKanbanOuter,
  planKanbanGrouping,
  taskInKanbanGroup,
  type KanbanGroupContext,
} from "./kanban-grouping";
import type { Task } from "@/components/kanban-card";

const context: KanbanGroupContext = {
  workflowName: "Dev",
  repositoryNames: new Map([
    ["r1", "web"],
    ["r2", "api"],
  ]),
  repositoryGroups: [
    { id: "g-b", name: "Beta", repositoryIds: ["r2"] },
    { id: "g-a", name: "Alpha", repositoryIds: ["r1"] },
  ],
  stateLabel: (state) => state ?? "No state",
  priorityLabel: (priority) => priority ?? "No priority",
  ungroupedLabel: "Ungrouped",
};

function task(id: string, repositoryId: string | null, state: Task["state"] = "TODO"): Task {
  return {
    id,
    title: id,
    workflowStepId: "s1",
    state,
    repositories: repositoryId ? [{ repository_id: repositoryId, position: 0 }] : [],
  } as unknown as Task;
}

describe("planKanbanGrouping", () => {
  it("renders no groups for none", () => {
    expect(planKanbanGrouping(["none"])).toEqual({ outer: null, laneKeys: [] });
  });

  it("makes the first non-workflow key the global outer level", () => {
    expect(planKanbanGrouping(["repositoryGroup"])).toEqual({
      outer: "repositoryGroup",
      laneKeys: [],
    });
  });

  it("nests workflows under the outer key without a redundant workflow lane", () => {
    expect(planKanbanGrouping(["repositoryGroup", "workflow"])).toEqual({
      outer: "repositoryGroup",
      laneKeys: [],
    });
    expect(planKanbanGrouping(["repositoryGroup", "workflow", "state"])).toEqual({
      outer: "repositoryGroup",
      laneKeys: ["state"],
    });
  });

  it("keeps workflows outermost when workflow is the first key", () => {
    expect(planKanbanGrouping(["workflow", "priority"])).toEqual({
      outer: null,
      laneKeys: ["priority"],
    });
  });
});

describe("groupKanbanOuter", () => {
  // Tasks of two workflows, interleaved the way the board concatenates them.
  const tasks = [task("a", "r2"), task("b", null), task("c", "r1"), task("d", "r2")];

  it("lists each repository group once across workflows, ungrouped last", () => {
    const groups = groupKanbanOuter(tasks, "repositoryGroup", context);
    expect(groups.map((group) => group.label)).toEqual(["Alpha", "Beta", "Ungrouped"]);
    expect(groups.map((group) => group.items.map((item) => item.id))).toEqual([
      ["c"],
      ["a", "d"],
      ["b"],
    ]);
  });

  it("keeps group order and identity when a filter removes tasks", () => {
    const full = groupKanbanOuter(tasks, "repositoryGroup", context);
    const filtered = groupKanbanOuter(
      tasks.filter((item) => item.id !== "c"),
      "repositoryGroup",
      context,
    );
    expect(filtered.map((group) => group.path)).toEqual([full[1].path, full[2].path]);
  });

  it("scopes a workflow board to exactly its outer group's tasks", () => {
    const [alpha] = groupKanbanOuter(tasks, "repositoryGroup", context);
    const scoped = tasks.filter((item) =>
      taskInKanbanGroup(item, "repositoryGroup", alpha.key, context),
    );
    expect(scoped.map((item) => item.id)).toEqual(["c"]);
  });
});

describe("groupKanbanLanes", () => {
  it("flattens nested keys into one lane per innermost group with a unique identity", () => {
    const lanes = groupKanbanLanes(
      [task("a", "r1", "TODO"), task("b", "r1", "REVIEW"), task("c", "r2", "TODO")],
      ["repository", "state"],
      context,
    );
    expect(lanes.map((lane) => lane.label)).toEqual(["web / TODO", "web / REVIEW", "api / TODO"]);
    expect(new Set(lanes.map((lane) => lane.key)).size).toBe(3);
  });
});
