"use client";

import { useCallback, useMemo } from "react";
import { type DragEndEvent, PointerSensor, useSensor, useSensors } from "@dnd-kit/core";
import { arrayMove } from "@dnd-kit/sortable";
import { useAppStore } from "@/components/state-provider";
import type { Task } from "@/components/kanban-card";
import { useKanbanOuterGroups } from "@/hooks/domains/kanban/use-workflow-group-lanes";
import { useRepositorySets } from "@/hooks/domains/workspace/use-repository-sets";
import { reorderWorkflows } from "@/lib/api";
import { deriveAutoHiddenStepIds } from "@/lib/kanban/auto-hide-empty-columns";
import { planKanbanGrouping } from "@/lib/kanban/kanban-grouping";
import { sortWorkflowStepsByPosition } from "@/lib/kanban/workflow-step-order";
import { useStableStringSet } from "@/hooks/domains/kanban/use-swimlane-render-data";
import type { WorkflowSnapshotData } from "@/lib/state/slices/kanban/types";
import {
  buildTaskVcsSearchIndex,
  getTaskPRsByTaskIdForCurrentWorkspace,
} from "@/lib/kanban/task-search-index";
import type { ViewGroupKey } from "@/lib/view-model/types";

const WORKFLOW_POINTER_SENSOR_OPTIONS = { activationConstraint: { distance: 8 } };
/** Stable identity so a workspace with no GitLab MRs does not re-trigger downstream memos. */
const EMPTY_TASK_MRS_BY_TASK_ID: Record<string, never[]> = {};

export function useWorkflowReorder(
  orderedWorkflows: { id: string; name: string }[],
  workflowFilter: string | null,
) {
  const reorderWorkflowItems = useAppStore((state) => state.reorderWorkflowItems);
  const workflows = useAppStore((state) => state.workflows.items);
  const workspaceId = workflows[0]?.workspaceId;
  const sensors = useSensors(useSensor(PointerSensor, WORKFLOW_POINTER_SENSOR_OPTIONS));
  const canSort = !workflowFilter && orderedWorkflows.length > 1;

  const handleDragEnd = useCallback(
    (event: DragEndEvent) => {
      const { active, over } = event;
      if (!over || active.id === over.id) return;
      const oldIndex = orderedWorkflows.findIndex((wf) => wf.id === active.id);
      const newIndex = orderedWorkflows.findIndex((wf) => wf.id === over.id);
      if (oldIndex === -1 || newIndex === -1) return;
      const reordered = arrayMove(orderedWorkflows, oldIndex, newIndex);
      reorderWorkflowItems(reordered.map((wf) => wf.id));
      if (workspaceId) {
        reorderWorkflows(
          workspaceId,
          reordered.map((wf) => wf.id),
        ).catch(() => {});
      }
    },
    [orderedWorkflows, reorderWorkflowItems, workspaceId],
  );

  return { sensors, canSort, handleDragEnd };
}

/** One lowercase `#<number>` haystack per task, built from its linked PRs/MRs in the active workspace. */
export function useVcsSearchIndex() {
  const taskPRsByTaskId = useAppStore((state) =>
    getTaskPRsByTaskIdForCurrentWorkspace(
      state.taskPRs,
      state.workspaces.activeId,
      state.workspaceContextGeneration,
    ),
  );
  const activeWorkspaceId = useAppStore((state) => state.workspaces.activeId);
  const taskMRsByTaskId = useAppStore(
    (state) =>
      (activeWorkspaceId && state.taskMRs.byWorkspaceId[activeWorkspaceId]) ||
      EMPTY_TASK_MRS_BY_TASK_ID,
  );
  return useMemo(
    () => buildTaskVcsSearchIndex(taskPRsByTaskId, taskMRsByTaskId),
    [taskMRsByTaskId, taskPRsByTaskId],
  );
}

type BoardGroupingOptions = {
  groups: readonly ViewGroupKey[];
  isMobileKanban: boolean;
  renderedWorkflows: { id: string; name: string }[];
  getFilteredTasks: (workflowId: string) => Task[];
};

/**
 * Resolves the ordered group keys into the board's outer groups and per-board
 * lane keys. The phone board shows one workflow at a time, so there the outer
 * key becomes the first lane level instead of a section above the workflows.
 */
export function useBoardGrouping({
  groups,
  isMobileKanban,
  renderedWorkflows,
  getFilteredTasks,
}: BoardGroupingOptions) {
  // The group catalog is only hydrated at boot on some routes; load it here
  // once for the board, so repository-group lanes never fall back to one
  // "ungrouped" bucket for lack of data.
  useRepositorySets(useAppStore((state) => state.workspaces?.activeId ?? null));
  const plan = useMemo(() => planKanbanGrouping(groups), [groups]);
  const outerKey = isMobileKanban ? null : plan.outer;
  const laneKeys = useMemo(
    () => (isMobileKanban && plan.outer ? [plan.outer, ...plan.laneKeys] : plan.laneKeys),
    [isMobileKanban, plan],
  );
  const allTasks = useMemo(
    () => (outerKey ? renderedWorkflows.flatMap((wf) => getFilteredTasks(wf.id)) : []),
    [getFilteredTasks, outerKey, renderedWorkflows],
  );
  const outerGroups = useKanbanOuterGroups(allTasks, outerKey);
  return { outerKey, laneKeys, outerGroups };
}

type WorkflowSteps = WorkflowSnapshotData["steps"];

/**
 * A workflow board's columns: the steps the user has not hidden are move
 * targets, and of those the ones not auto-hidden for being empty are rendered.
 */
export function useVisibleWorkflowSteps(
  snapshotSteps: WorkflowSteps,
  occupancyTasks: Parameters<typeof deriveAutoHiddenStepIds>[1],
  autoHideEmpty: boolean,
  hiddenStepIds: Parameters<typeof deriveAutoHiddenStepIds>[3],
  hiddenSet: ReadonlySet<string>,
) {
  const derivedAutoHiddenSet = useMemo(
    () => deriveAutoHiddenStepIds(snapshotSteps, occupancyTasks, autoHideEmpty, hiddenStepIds),
    [autoHideEmpty, hiddenStepIds, occupancyTasks, snapshotSteps],
  );
  const autoHiddenSet = useStableStringSet(derivedAutoHiddenSet);
  const moveTargetSteps = useMemo(
    () => sortWorkflowStepsByPosition(snapshotSteps).filter((step) => !hiddenSet.has(step.id)),
    [hiddenSet, snapshotSteps],
  );
  const steps = useMemo(
    () => moveTargetSteps.filter((step) => !autoHiddenSet.has(step.id)),
    [moveTargetSteps, autoHiddenSet],
  );
  return { moveTargetSteps, steps };
}
