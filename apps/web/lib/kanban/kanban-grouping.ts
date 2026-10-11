import { applyViewGroup, applyViewGroupPath } from "@/lib/view-model/groups";
import type {
  GroupedViewItems,
  ViewGroup,
  ViewGroupKey,
  ViewGroupNode,
} from "@/lib/view-model/types";
import {
  repositoryGroupKeyAndLabel,
  UNGROUPED_REPOSITORY_GROUP_KEY,
} from "@/lib/view-model/repository-group";
import type { RepositoryGroup } from "@/lib/view-model/repository-group";
import type { Task } from "@/components/kanban-card";

/** The task's primary repository id: the lowest-position linked repository. */
function primaryRepositoryId(task: Task): string | undefined {
  const linked = task.repositories;
  if (!linked || linked.length === 0) return undefined;
  return linked.reduce((first, repo) => (repo.position < first.position ? repo : first))
    .repository_id;
}

/**
 * Everything the Kanban group extractor needs from the surrounding view: the
 * workflow's display name and the label resolvers for repository, state and
 * priority values. Pure and synchronous so it is unit-testable.
 */
export type KanbanGroupContext = {
  workflowName: string;
  repositoryNames: Map<string, string>;
  repositoryGroups: readonly RepositoryGroup[];
  stateLabel: (state: Task["state"]) => string;
  priorityLabel: (priority: Task["priority"]) => string;
  ungroupedLabel: string;
};

/**
 * Maps one board card to its group key/label for the active group key. Uses the
 * D7 repository-group semantics (first group in set order holding the task's
 * first repository, else Ungrouped).
 */
export function extractKanbanGroup(
  task: Task,
  groupKey: ViewGroupKey,
  context: KanbanGroupContext,
): { key: string; label: string } {
  switch (groupKey) {
    case "workflow":
      return { key: "workflow", label: context.workflowName };
    case "repository": {
      const repoId = primaryRepositoryId(task);
      if (!repoId) return { key: "repository:none", label: context.ungroupedLabel };
      return {
        key: `repository:${repoId}`,
        label: context.repositoryNames.get(repoId) ?? repoId,
      };
    }
    case "repositoryGroup": {
      const ids = (task.repositories ?? []).map((repo) => String(repo.repository_id));
      const { key, label } = repositoryGroupKeyAndLabel(
        ids,
        context.repositoryGroups,
        context.ungroupedLabel,
      );
      return { key, label };
    }
    case "state":
      return { key: `state:${task.state ?? "none"}`, label: context.stateLabel(task.state) };
    case "priority":
      return {
        key: `priority:${task.priority ?? "none"}`,
        label: context.priorityLabel(task.priority),
      };
    default:
      return { key: "all", label: "" };
  }
}

/** Partitions board cards into the active group's lanes through the shared engine. */
export function groupKanbanTasks(
  tasks: Task[],
  groupKey: ViewGroupKey,
  context: KanbanGroupContext,
): GroupedViewItems<Task> {
  return applyViewGroup(tasks, groupKey, (task) => extractKanbanGroup(task, groupKey, context));
}

/**
 * How an ordered group list maps onto the board. A lane's columns are one
 * workflow's steps, so the workflow is always a structural level: `outer`
 * partitions tasks once across every workflow, each workflow then renders
 * under its outer group, and `laneKeys` subdivide a workflow's board.
 */
export type KanbanGroupPlan = {
  outer: ViewGroupKey | null;
  laneKeys: ViewGroupKey[];
};

export function planKanbanGrouping(order: readonly ViewGroupKey[]): KanbanGroupPlan {
  const keys = order.filter((key) => key !== "none");
  // ponytail: the workflow level always sits directly under the outer group,
  // wherever it appears in the list; honour its position if lanes ever need to
  // wrap several workflows.
  const laneKeys = keys.slice(1).filter((key) => key !== "workflow");
  if (keys.length === 0) return { outer: null, laneKeys: [] };
  if (keys[0] === "workflow") return { outer: null, laneKeys };
  return { outer: keys[0], laneKeys };
}

const UNGROUPED_KEYS = new Set([
  "repository:none",
  "state:none",
  "priority:none",
  UNGROUPED_REPOSITORY_GROUP_KEY,
]);

/**
 * The outer groups across every workflow's tasks. Order does not depend on
 * which tasks a filter leaves visible: named groups sort by label then key and
 * the ungrouped group is last.
 */
export function groupKanbanOuter(
  tasks: Task[],
  groupKey: ViewGroupKey,
  context: KanbanGroupContext,
): ViewGroupNode<Task>[] {
  const nodes = applyViewGroupPath(tasks, [
    { groupKey, extract: (task) => extractKanbanGroup(task, groupKey, context) },
  ]);
  return nodes.sort((a, b) => {
    const ungrouped = Number(UNGROUPED_KEYS.has(a.key)) - Number(UNGROUPED_KEYS.has(b.key));
    if (ungrouped !== 0) return ungrouped;
    return a.label.localeCompare(b.label) || a.key.localeCompare(b.key);
  });
}

/** True when a task falls in the outer group `key` for `groupKey`. */
export function taskInKanbanGroup(
  task: Task,
  groupKey: ViewGroupKey,
  key: string,
  context: KanbanGroupContext,
): boolean {
  return extractKanbanGroup(task, groupKey, context).key === key;
}

/**
 * One workflow board's lanes for the keys below the workflow level. Nested
 * keys flatten to one lane per innermost group; the lane key is the full path
 * and its label joins the levels, so two lanes never share an identity.
 */
export function groupKanbanLanes(
  tasks: Task[],
  laneKeys: readonly ViewGroupKey[],
  context: KanbanGroupContext,
): ViewGroup<Task>[] {
  const levels = laneKeys.map((groupKey) => ({
    groupKey,
    extract: (task: Task) => extractKanbanGroup(task, groupKey, context),
  }));
  const lanes: ViewGroup<Task>[] = [];
  const walk = (nodes: ViewGroupNode<Task>[], labels: string[]) => {
    for (const node of nodes) {
      const path = [...labels, node.label];
      if (node.children.length === 0) {
        lanes.push({ key: node.path, label: path.join(" / "), items: node.items });
      } else {
        walk(node.children, path);
      }
    }
  };
  walk(applyViewGroupPath(tasks, levels), []);
  return lanes;
}

export { UNGROUPED_REPOSITORY_GROUP_KEY };
