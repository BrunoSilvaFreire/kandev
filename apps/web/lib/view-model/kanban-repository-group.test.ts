import { describe, expect, it } from "vitest";
import { applyKanbanFilterClauses } from "./kanban";
import type { KanbanFilterableTask } from "./kanban";
import type { ViewFilterClause } from "./types";

const groups = [
  { id: "cityscape", name: "Cityscape", repositoryIds: ["r1", "r2"] },
  { id: "kandev", name: "Kandev", repositoryIds: ["r3"] },
];

type Task = KanbanFilterableTask & {
  repositories?: Array<{ repository_id: string }>;
};

const tasks: Task[] = [
  { id: "a", repositoryId: "r1", repositories: [{ repository_id: "r1" }, { repository_id: "r2" }] },
  { id: "b", repositoryId: "r3", repositories: [{ repository_id: "r3" }] },
  { id: "c" },
];

function clause(
  dimension: "repositoryGroup" | "repository",
  value: string | string[],
): ViewFilterClause<"repositoryGroup" | "repository"> {
  return { id: "c", dimension, op: "in", value };
}

describe("applyKanbanFilterClauses repositoryGroup", () => {
  it("matches a multi-repo task through any of its repositories", () => {
    const result = applyKanbanFilterClauses(tasks, [clause("repositoryGroup", ["cityscape"])], {
      repositoryGroups: groups,
    });
    expect(result.map((task) => task.id)).toEqual(["a"]);
  });

  it("matches several selected groups (OR within a clause)", () => {
    const result = applyKanbanFilterClauses(
      tasks,
      [clause("repositoryGroup", ["cityscape", "kandev"])],
      { repositoryGroups: groups },
    );
    expect(result.map((task) => task.id)).toEqual(["a", "b"]);
  });

  it("excludes tasks with no repository", () => {
    const result = applyKanbanFilterClauses(tasks, [clause("repositoryGroup", ["cityscape"])], {
      repositoryGroups: groups,
    });
    expect(result.some((task) => task.id === "c")).toBe(false);
  });
});
