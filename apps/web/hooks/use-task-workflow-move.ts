"use client";

import { useReportTransitionFailure } from "@/hooks/domains/task/use-task-transition-failure";
import { useCallback } from "react";
import { useTranslation } from "react-i18next";
import { bulkMoveSelectedTasks } from "@/lib/api";
import { useToast } from "@/components/toast-provider";
import { getTaskMoveErrorMessage } from "@/components/task/task-move-error-message";

export function useTaskWorkflowMove() {
  const { toast } = useToast();
  const reportFailure = useReportTransitionFailure();
  const { t } = useTranslation("task");

  return useCallback(
    async (
      taskIds: string[],
      targetWorkflowId: string,
      targetStepId: string,
      destination: "step" | "workflow" = "workflow",
    ) => {
      const ids = [...new Set(taskIds.filter(Boolean))];
      if (ids.length === 0) return;
      try {
        const result = await bulkMoveSelectedTasks({
          task_ids: ids,
          target_workflow_id: targetWorkflowId,
          target_step_id: targetStepId,
        });
        toast({
          title: t(destination === "step" ? "task:movedTasksToStep" : "task:movedTasksToWorkflow", {
            count: result.moved_count,
          }),
          description: t(
            destination === "step"
              ? "task:movedTasksStepDescription"
              : "task:movedTasksWorkflowDescription",
            { count: result.moved_count },
          ),
          variant: "success",
        });
      } catch (error) {
        const fallback = t("task:taskMoveErrorGeneric");
        reportFailure(error);
        toast({
          title: t("task:failedToMoveTask"),
          description: getTaskMoveErrorMessage(error, fallback, t),
          variant: "error",
        });
        throw error;
      }
    },
    [reportFailure, t, toast],
  );
}
