import { applyViewGroup } from "@/lib/view-model/groups";
import type { GroupedViewItems, ViewGroupKey } from "@/lib/view-model/types";
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

export { UNGROUPED_REPOSITORY_GROUP_KEY };
