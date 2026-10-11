import { describe, expect, it } from "vitest";
import type { Task } from "@/lib/types/http";
import { buildNestedTaskSections } from "./task-list-sections";

function task(id: string, workflow: string, repositoryId: string | null): Task {
  return {
    id,
    title: id,
    workflow_id: workflow,
    workflow_step_id: "s1",
    state: "REVIEW",
    repositories: repositoryId ? [{ repository_id: repositoryId, position: 0 }] : [],
  } as unknown as Task;
}

const lookups = {
  workflowMap: new Map([
    ["wf-a", "First"],
    ["wf-b", "Second"],
  ]),
  repoMap: new Map<string, string>(),
  repositoryGroups: [
    { id: "g1", name: "Alpha", repositoryIds: ["r1"] },
    { id: "g2", name: "Beta", repositoryIds: ["r2"] },
  ],
};
const tasks = [
  task("1", "wf-a", "r1"),
  task("2", "wf-b", "r1"),
  task("3", "wf-a", "r2"),
  task("4", "wf-a", null),
];

describe("buildNestedTaskSections", () => {
  it("renders one outer heading per group with its workflows nested beneath", () => {
    const sections = buildNestedTaskSections(tasks, ["repositoryGroup", "workflow"], lookups);
    expect(
      sections.map((section) => [
        section.level,
        section.title,
        section.count ?? section.nodes.map((node) => node.task.id),
      ]),
    ).toEqual([
      [0, "Alpha", 2],
      [1, "First", ["1"]],
      [1, "Second", ["2"]],
      [0, "Beta", 1],
      [1, "First", ["3"]],
      [0, expect.any(String), 1],
      [1, "First", ["4"]],
    ]);
  });

  it("gives every section a unique path identity, including repeated workflow names", () => {
    const sections = buildNestedTaskSections(tasks, ["repositoryGroup", "workflow"], lookups);
    expect(new Set(sections.map((section) => section.key)).size).toBe(sections.length);
  });

  it("carries the rows on a single level without heading-only sections", () => {
    const sections = buildNestedTaskSections(tasks, ["repositoryGroup"], lookups);
    expect(sections.map((section) => section.count)).toEqual([undefined, undefined, undefined]);
    expect(sections.map((section) => section.nodes.length)).toEqual([2, 1, 1]);
  });
});
