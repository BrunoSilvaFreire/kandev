"use client";

import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import type { Task } from "@/components/kanban-card";
import { useKanbanDisplaySettings } from "@/hooks/use-kanban-display-settings";
import { useRepositoryGroups } from "@/hooks/use-repository-groups";
import {
  groupKanbanLanes,
  groupKanbanOuter,
  taskInKanbanGroup,
  type KanbanGroupContext,
} from "@/lib/kanban/kanban-grouping";
import { formatTaskStateLabel } from "@/lib/ui/state-labels";
import { TASK_PRIORITY_LABEL_KEYS } from "@/lib/tasks/task-priority";
import type { ViewGroup, ViewGroupKey, ViewGroupNode } from "@/lib/view-model/types";

/** The outer group a workflow board is rendered under, when there is one. */
export type KanbanGroupScope = { groupKey: ViewGroupKey; key: string };

/**
 * The label resolvers every Kanban group extractor shares: the Repository
 * Group catalog and the repository, state, and priority names.
 */
export function useKanbanGroupContext(workflowName = ""): KanbanGroupContext {
  const { t, i18n } = useTranslation();
  const display = useKanbanDisplaySettings();
  const repositoryGroups = useRepositoryGroups();
  const repositoryNames = useMemo(
    () => new Map(display.repositories.map((repo) => [String(repo.id), repo.name])),
    [display.repositories],
  );
  const ungroupedLabel = t("tasks:ungrouped");
  return useMemo(
    () => ({
      workflowName,
      repositoryNames,
      repositoryGroups,
      stateLabel: (state) => (state ? formatTaskStateLabel(state) : ungroupedLabel),
      priorityLabel: (priority) =>
        priority ? t(TASK_PRIORITY_LABEL_KEYS[priority]) : ungroupedLabel,
      ungroupedLabel,
    }),
    // i18n.language re-resolves the labels on a locale switch.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [workflowName, repositoryNames, repositoryGroups, ungroupedLabel, t, i18n.language],
  );
}

/** The outer groups across every visible workflow's tasks. */
export function useKanbanOuterGroups(
  tasks: Task[],
  groupKey: ViewGroupKey | null,
): ViewGroupNode<Task>[] {
  const context = useKanbanGroupContext();
  return useMemo(
    () => (groupKey ? groupKanbanOuter(tasks, groupKey, context) : []),
    [context, groupKey, tasks],
  );
}

/**
 * One workflow board's tasks and lanes. The tasks are narrowed to the outer
 * group when the board is rendered under one; lanes are `null` when no key
 * subdivides the board.
 */
export function useWorkflowGroupLanes(
  wf: { id: string; name: string },
  tasks: Task[],
  laneKeys: readonly ViewGroupKey[],
  scope?: KanbanGroupScope,
): { tasks: Task[]; lanes: ViewGroup<Task>[] | null } {
  const context = useKanbanGroupContext(wf.name);
  const scopeGroupKey = scope?.groupKey;
  const scopeKey = scope?.key;
  return useMemo(() => {
    const scoped =
      scopeGroupKey && scopeKey !== undefined
        ? tasks.filter((task) => taskInKanbanGroup(task, scopeGroupKey, scopeKey, context))
        : tasks;
    return {
      tasks: scoped,
      lanes: laneKeys.length > 0 ? groupKanbanLanes(scoped, laneKeys, context) : null,
    };
  }, [context, laneKeys, scopeGroupKey, scopeKey, tasks]);
}
