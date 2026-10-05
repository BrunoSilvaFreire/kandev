"use client";

import { useMemo } from "react";
import { useOptionalAppStore } from "@/components/state-provider";
import type { KanbanState } from "@/lib/state/slices";
import { sortWorkflowStepsByPosition } from "@/lib/kanban/workflow-step-order";
import { WorkflowTransitionDisclosure } from "../workflow-transition-disclosure";
import type { WorkflowStepperStep } from "../workflow-step-disclosure";

const EMPTY_STEPS: KanbanState["steps"] = [];

function toStepperStep(step: KanbanState["steps"][number]): WorkflowStepperStep {
  return {
    id: step.id,
    name: step.title,
    color: step.color,
    position: step.position,
    events: step.events,
    allow_manual_move: step.allow_manual_move,
    is_start_step: step.is_start_step,
    agent_profile_id: step.agent_profile_id,
  };
}

/**
 * Phone entry point for the expanded workflow transitions. It reuses the same
 * disclosure: on a coarse pointer the control is >=44px and opens the existing
 * touch drawer, never squeezing a graph into the narrow header. Reads the
 * optional store so it is inert outside a StateProvider (tests, shells).
 */
export function MobileWorkflowTransitionButton({ taskId }: { taskId: string | null }) {
  const task = useOptionalAppStore(
    (state) =>
      taskId ? (state.kanban.tasks.find((candidate) => candidate.id === taskId) ?? null) : null,
    null,
  );
  const rawSteps = useOptionalAppStore((state) => {
    if (!taskId) return EMPTY_STEPS;
    const task = state.kanban.tasks.find((candidate) => candidate.id === taskId);
    if (!task) return EMPTY_STEPS;
    const active =
      state.kanban.workflowId === task.workflowId
        ? state.kanban.steps
        : state.kanbanMulti.snapshots[task.workflowId]?.steps;
    return active ?? EMPTY_STEPS;
  }, EMPTY_STEPS);
  const steps = useMemo(() => sortWorkflowStepsByPosition(rawSteps.map(toStepperStep)), [rawSteps]);
  if (!taskId || steps.length === 0) return null;
  return (
    <WorkflowTransitionDisclosure
      steps={steps}
      currentStepId={task?.workflowStepId ?? null}
      taskId={taskId}
    />
  );
}
