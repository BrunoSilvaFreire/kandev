"use client";

import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import type { Task } from "@/components/kanban-card";
import { useKanbanDisplaySettings } from "@/hooks/use-kanban-display-settings";
import { useRepositoryGroups } from "@/hooks/use-repository-groups";
import { groupKanbanTasks } from "@/lib/kanban/kanban-grouping";
import { formatTaskStateLabel } from "@/lib/ui/state-labels";
import { TASK_PRIORITY_LABEL_KEYS } from "@/lib/tasks/task-priority";
import type { ViewGroup } from "@/lib/view-model/types";

/**
 * The active group's lanes for one workflow's board, or `null` when grouping is
 * off. Reads the shared persisted group key and the Repository Group catalog so
 * every lane uses the same semantics as the other Home views.
 */
export function useWorkflowGroupLanes(
  wf: { id: string; name: string },
  tasks: Task[],
): ViewGroup<Task>[] | null {
  const { t, i18n } = useTranslation();
  const display = useKanbanDisplaySettings();
  const repositoryGroups = useRepositoryGroups();
  const repositoryNames = useMemo(
    () => new Map(display.repositories.map((repo) => [String(repo.id), repo.name])),
    [display.repositories],
  );
  const groupKey = display.group;
  const ungroupedLabel = t("tasks:ungrouped");

  return useMemo(() => {
    if (groupKey === "none") return null;
    return groupKanbanTasks(tasks, groupKey, {
      workflowName: wf.name,
      repositoryNames,
      repositoryGroups,
      stateLabel: (state) => (state ? formatTaskStateLabel(state) : ungroupedLabel),
      priorityLabel: (priority) =>
        priority ? t(TASK_PRIORITY_LABEL_KEYS[priority]) : ungroupedLabel,
      ungroupedLabel,
    }).groups;
  }, [
    groupKey,
    tasks,
    wf.name,
    repositoryNames,
    repositoryGroups,
    ungroupedLabel,
    t,
    i18n.language,
  ]);
}
