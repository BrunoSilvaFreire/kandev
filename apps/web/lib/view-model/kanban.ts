import type { TaskPriority, TaskState } from "@/lib/types/http";
import {
  applyViewFilters,
  type ViewFilterClause,
  type ViewDimensionContext,
  type ViewValueAccessor,
} from "./index";

/** The task fields the Kanban/List filter dimensions read. */
export type KanbanFilterableTask = {
  id: string;
  repositoryId?: string;
  repositoryIds?: string[];
  /** Multi-repo chips; used to derive repository membership when `repositoryIds` is absent. */
  repositories?: Array<{ repository_id: string }>;
  workflowId?: string;
  workflowStepId?: string;
  state?: TaskState;
  priority?: TaskPriority;
};

export type KanbanFilterDimension =
  | "repository"
  | "repositoryGroup"
  | "workflow"
  | "workflowStep"
  | "state"
  | "priority";

/**
 * Builds shared clauses from the legacy Kanban/List display preferences.
 * This is the read-once side of the migration: existing `repository_ids` and
 * priority-token selections are expressed as clauses, and callers that already
 * have persisted clauses should use those instead.
 */
export function buildKanbanFilterClauses(
  repositoryIds: Iterable<string>,
  priorityTokens: readonly TaskPriority[] | undefined,
): ViewFilterClause<KanbanFilterDimension>[] {
  const clauses: ViewFilterClause<KanbanFilterDimension>[] = [];
  const repositories = [...repositoryIds];
  if (repositories.length > 0) {
    clauses.push({
      id: "kanban:repository",
      dimension: "repository",
      op: "in",
      value: repositories,
    });
  }
  if (priorityTokens && priorityTokens.length > 0) {
    clauses.push({
      id: "kanban:priority",
      dimension: "priority",
      op: "in",
      value: [...priorityTokens],
    });
  }
  return clauses;
}

/**
 * The view's active clauses: persisted clauses win; otherwise the legacy
 * `repository_ids` / priority tokens are read once into the shared form.
 */
export function resolveKanbanFilterClauses(
  persisted: readonly ViewFilterClause[] | undefined,
  repositoryIds: Iterable<string>,
  priorityTokens: readonly TaskPriority[] | undefined,
): ViewFilterClause<KanbanFilterDimension>[] {
  if (persisted && persisted.length > 0) {
    return [...persisted] as ViewFilterClause<KanbanFilterDimension>[];
  }
  return buildKanbanFilterClauses(repositoryIds, priorityTokens);
}

const kanbanValueAccessor: ViewValueAccessor<KanbanFilterableTask, KanbanFilterDimension> = (
  task,
  dimension,
) => {
  switch (dimension) {
    case "repository":
      return (
        task.repositories?.map((repository) => repository.repository_id) ??
        (task.repositoryId ? [task.repositoryId] : [])
      );
    case "repositoryGroup":
      return task.repositoryIds ?? [];
    case "workflow":
      return task.workflowId;
    case "workflowStep":
      return task.workflowStepId;
    case "state":
      return task.state;
    case "priority":
      return task.priority;
  }
};

/** Applies the shared filter engine to Kanban/List tasks. */
export function applyKanbanViewFilters<T extends KanbanFilterableTask>(
  tasks: T[],
  repositoryIds: Iterable<string>,
  priorityTokens: readonly TaskPriority[] | undefined,
): T[] {
  return applyKanbanFilterClauses(tasks, buildKanbanFilterClauses(repositoryIds, priorityTokens));
}

/** Applies persisted (or otherwise resolved) shared clauses to Kanban/List tasks. */
export function applyKanbanFilterClauses<T extends KanbanFilterableTask>(
  tasks: T[],
  clauses: readonly ViewFilterClause<KanbanFilterDimension>[],
  context?: ViewDimensionContext,
): T[] {
  if (clauses.length === 0) return tasks;
  const accessor: ViewValueAccessor<T, KanbanFilterDimension> = (task, dimension) => {
    if (dimension === "repositoryGroup") {
      const ids = task.repositoryIds ?? (task.repositories ?? []).map((r) => r.repository_id);
      const groups = context?.repositoryGroups ?? [];
      return groups
        .filter((group) => ids.some((id) => group.repositoryIds.includes(id)))
        .map((group) => group.id);
    }
    return kanbanValueAccessor(task, dimension);
  };
  return applyViewFilters(tasks, [...clauses], accessor);
}
