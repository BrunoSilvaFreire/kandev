"use client";

import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { useAppStore } from "@/components/state-provider";
import { deriveWorkflowTransitionEdges } from "@/lib/task/workflow-transition-edges";
import { canMoveToStep } from "./workflow-step-disclosure";

export const HANDOFF_STAY_IN_STEP = "__stay__";

/**
 * Destination-step picker for a same-task handoff. Options are the current
 * step's named transitions plus every destination the normal move eligibility
 * (`canMoveToStep`) allows. The default is to stay in the current step, so a
 * plain handoff keeps its existing behavior.
 */
export function HandoffStepSelect({
  taskId,
  value,
  onChange,
}: {
  taskId: string;
  value: string;
  onChange: (value: string) => void;
}) {
  const { t } = useTranslation();
  const steps = useAppStore((state) => state.kanban.steps);
  const workflowId = useAppStore((state) => state.kanban.workflowId);
  const currentStepId = useAppStore(
    (state) => state.kanban.tasks.find((task) => task.id === taskId)?.workflowStepId ?? null,
  );

  const options = useMemo(() => {
    const sorted = [...steps].sort((left, right) => left.position - right.position);
    const stepperSteps = sorted.map((step) => ({
      id: step.id,
      name: step.title,
      color: step.color,
      position: step.position,
      events: step.events,
      allow_manual_move: step.allow_manual_move,
    }));
    const currentIndex = sorted.findIndex((step) => step.id === currentStepId);
    const namedTransitions = new Set(
      deriveWorkflowTransitionEdges(stepperSteps, currentStepId)
        .filter((edge) => edge.fromStepId === currentStepId && !edge.manual)
        .map((edge) => edge.toStepId),
    );
    return sorted.filter((step) => {
      if (step.id === currentStepId) return false;
      if (namedTransitions.has(step.id)) return true;
      const index = sorted.indexOf(step);
      return canMoveToStep({
        isArchived: false,
        isCurrent: false,
        taskId,
        workflowId,
        isAdjacent: currentIndex >= 0 && Math.abs(index - currentIndex) === 1,
        allowManualMove: step.allow_manual_move,
      });
    });
  }, [currentStepId, steps, taskId, workflowId]);

  return (
    <div className="space-y-1">
      <label className="text-xs font-medium" htmlFor="handoff-destination-step">
        {t("task:handoffDestinationStep")}
      </label>
      <Select value={value} onValueChange={onChange}>
        <SelectTrigger id="handoff-destination-step" data-testid="handoff-destination-step">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={HANDOFF_STAY_IN_STEP}>{t("task:handoffStayInCurrentStep")}</SelectItem>
          {options.map((step) => (
            <SelectItem key={step.id} value={step.id}>
              {step.title}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}
