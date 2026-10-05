import type { Task } from "@/components/kanban-card";
import { mapSelectedRepositoryIds } from "@/lib/kanban/filters";
import { applyKanbanFilterClauses, applyKanbanViewFilters } from "@/lib/view-model/kanban";
import type { KanbanFilterDimension } from "@/lib/view-model/kanban";
import type { ViewFilterClause } from "@/lib/view-model/types";
import type { RepositoryGroup } from "@/lib/view-model/repository-group";
import { changeRequestSearchText } from "@/lib/kanban/task-search-index";
import type { TaskPriority } from "@/lib/types/http";

type FilterTasksOptions = {
  searchQuery?: string;
  vcsSearchTextByTaskId?: Record<string, string>;
  matchesPluginTaskFilters?: (taskId: string) => boolean;
  hiddenStepIds?: Set<string>;
  /** A view lens like search: excluded from `occupancyTasks` in `projectWorkflowTasks`. */
  priorityFilterTokens?: TaskPriority[];
  /** Persisted shared filter clauses; when present they replace the legacy lens. */
  clauses?: readonly ViewFilterClause<KanbanFilterDimension>[];
  /** Ordered repository groups backing the `repositoryGroup` dimension. */
  repositoryGroups?: readonly RepositoryGroup[];
};

export function filterTasks(
  snapshots: Record<string, { tasks: Task[]; steps: { id: string }[] }>,
  workflowId: string,
  repoFilter: ReturnType<typeof mapSelectedRepositoryIds>,
  options?: FilterTasksOptions,
): Task[] {
  const snapshot = snapshots[workflowId];
  if (!snapshot) return [];
  let tasks = snapshot.tasks;
  const {
    hiddenStepIds,
    searchQuery,
    vcsSearchTextByTaskId,
    matchesPluginTaskFilters,
    priorityFilterTokens,
    clauses,
    repositoryGroups,
  } = options ?? {};
  if (hiddenStepIds && hiddenStepIds.size > 0) {
    const liveStepIds = new Set(snapshot.steps.map((step) => step.id));
    const effectiveHidden = new Set([...hiddenStepIds].filter((id) => liveStepIds.has(id)));
    if (effectiveHidden.size > 0) {
      tasks = tasks.filter((task) => !effectiveHidden.has(task.workflowStepId));
    }
  }
  // Persisted shared clauses win; otherwise the legacy repository/priority lens.
  if (clauses && clauses.length > 0) {
    tasks = applyKanbanFilterClauses(tasks, clauses, { repositoryGroups });
  } else {
    tasks = applyKanbanViewFilters(tasks, repoFilter, priorityFilterTokens);
  }
  if (searchQuery) {
    const query = searchQuery.toLowerCase();
    tasks = tasks.filter(
      (task) =>
        task.title.toLowerCase().includes(query) ||
        (task.description && task.description.toLowerCase().includes(query)) ||
        changeRequestSearchText(task).toLowerCase().includes(query) ||
        (vcsSearchTextByTaskId?.[task.id]?.toLowerCase().includes(query) ?? false),
    );
  }
  if (matchesPluginTaskFilters) {
    tasks = tasks.filter((task) => matchesPluginTaskFilters(task.id));
  }
  return tasks;
}

type WorkflowTaskProjectionOptions = {
  searchQuery: string;
  vcsSearchTextByTaskId?: Record<string, string>;
  matchesPluginTaskFilters?: (taskId: string) => boolean;
  hiddenStepIds?: Set<string>;
  priorityFilterTokens?: TaskPriority[];
  clauses?: readonly ViewFilterClause<KanbanFilterDimension>[];
  repositoryGroups?: readonly RepositoryGroup[];
};

export function projectWorkflowTasks(
  snapshots: Record<string, { tasks: Task[]; steps: { id: string }[] }>,
  workflowId: string,
  repoFilter: ReturnType<typeof mapSelectedRepositoryIds>,
  options: WorkflowTaskProjectionOptions,
): { visibleTasks: Task[]; occupancyTasks: Task[] } {
  return {
    visibleTasks: filterTasks(snapshots, workflowId, repoFilter, options),
    occupancyTasks: filterTasks(snapshots, workflowId, repoFilter, {
      matchesPluginTaskFilters: options.matchesPluginTaskFilters,
    }),
  };
}
