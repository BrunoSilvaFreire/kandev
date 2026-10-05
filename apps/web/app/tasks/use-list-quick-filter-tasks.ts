"use client";

import { useMemo } from "react";
import { useAppStore } from "@/components/state-provider";
import { useRepositoryGroups } from "@/hooks/use-repository-groups";
import { applyKanbanFilterClauses, resolveKanbanFilterClauses } from "@/lib/view-model/kanban";
import type { Task } from "@/lib/types/http";

/**
 * Filters the List's loaded page through the shared view model. The persisted
 * `taskViewFilters.list` clauses (written by the shared Quick Filter bar) are
 * applied client-side; the server still owns pagination, so the reported total
 * stays the unfiltered server total.
 */
export function useListQuickFilterTasks(tasks: Task[]): Task[] {
  const storedListFilters = useAppStore((state) => state.userSettings.taskViewFilters?.list);
  const repositoryGroups = useRepositoryGroups();
  const listClauses = useMemo(
    () => resolveKanbanFilterClauses(storedListFilters, [], []),
    [storedListFilters],
  );
  return useMemo(
    () => applyKanbanFilterClauses(tasks, listClauses, { repositoryGroups }),
    [tasks, listClauses, repositoryGroups],
  );
}
