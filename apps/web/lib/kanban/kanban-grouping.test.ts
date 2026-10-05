import { describe, expect, it } from "vitest";
import type { Task } from "@/components/kanban-card";
import { groupKanbanTasks } from "./kanban-grouping";
import type { KanbanGroupContext } from "./kanban-grouping";

function task(partial: Partial<Task> & Pick<Task, "id">): Task {
  return { title: partial.id, workflowStepId: "step-1", ...partial };
}

const context: KanbanGroupContext = {
  workflowName: "Delivery",
  repositoryNames: new Map([
    ["r1", "kandev"],
    ["r3", "docs"],
  ]),
  repositoryGroups: [
    { id: "cityscape", name: "Cityscape", repositoryIds: ["r1", "r2"] },
    { id: "kandev", name: "Kandev", repositoryIds: ["r3"] },
  ],
  stateLabel: (state) => `state:${state ?? "none"}`,
  priorityLabel: (priority) => `priority:${priority ?? "none"}`,
  ungroupedLabel: "Ungrouped",
};

const tasks: Task[] = [
  task({ id: "a", repositories: [{ id: "x", repository_id: "r1", position: 0 }] }),
  task({ id: "b", repositories: [{ id: "y", repository_id: "r3", position: 0 }] }),
  task({ id: "c" }),
  task({
    id: "d",
    repositories: [
      { id: "z2", repository_id: "r2", position: 1 },
      { id: "z1", repository_id: "r1", position: 0 },
    ],
  }),
];

describe("groupKanbanTasks", () => {
  it("groups by Repository Group with first-match semantics", () => {
    const { groups } = groupKanbanTasks(tasks, "repositoryGroup", context);
    const byKey = new Map(groups.map((group) => [group.key, group]));
    expect(byKey.get("cityscape")?.items.map((item) => item.id)).toEqual(["a", "d"]);
    expect(byKey.get("kandev")?.items.map((item) => item.id)).toEqual(["b"]);
    expect(byKey.get("__ungrouped__")?.items.map((item) => item.id)).toEqual(["c"]);
  });

  it("groups by the primary repository label", () => {
    const { groups } = groupKanbanTasks(tasks, "repository", context);
    const byKey = new Map(groups.map((group) => [group.key, group]));
    expect(byKey.get("repository:r1")?.label).toBe("kandev");
    expect(byKey.get("repository:r3")?.label).toBe("docs");
    expect(byKey.get("repository:none")?.label).toBe("Ungrouped");
  });

  it("groups by state and priority through the label resolvers", () => {
    const stateLanes = groupKanbanTasks(
      [task({ id: "s", state: "REVIEW" })],
      "state",
      context,
    ).groups;
    expect(stateLanes[0].label).toBe("state:REVIEW");
    const priorityLanes = groupKanbanTasks(
      [task({ id: "p", priority: "high" })],
      "priority",
      context,
    ).groups;
    expect(priorityLanes[0].label).toBe("priority:high");
  });

  it("treats workflow grouping as a single identity lane", () => {
    const { groups } = groupKanbanTasks(tasks, "workflow", context);
    expect(groups).toHaveLength(1);
    expect(groups[0].label).toBe("Delivery");
  });

  it("keeps every item in one pass-through lane for none", () => {
    const { groups } = groupKanbanTasks(tasks, "none", context);
    expect(groups).toHaveLength(1);
    expect(groups[0].items).toHaveLength(tasks.length);
  });
});
