"use client";

import { useCallback } from "react";
import { useAppStore } from "@/components/state-provider";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { isSubjectEdited } from "@/lib/approval";
import { usePendingPlanComments } from "./use-pending-comments";

/**
 * useApprovalRequest supplies the live data an approval card needs: the task's
 * pending plan comments, whether the plan changed since the request, and an
 * Open-plan action. Task-scoped reads go through the active task.
 */
export function useApprovalRequest(requestCreatedAt: string | null | undefined) {
  const taskId = useAppStore((s) => s.tasks.activeTaskId);
  const planUpdatedAt = useAppStore((s) =>
    taskId ? (s.taskPlans.byTaskId[taskId]?.updated_at ?? null) : null,
  );
  const pendingComments = usePendingPlanComments(taskId);
  const addPlanPanel = useDockviewStore((s) => s.addPlanPanel);

  const openPlan = useCallback(() => {
    addPlanPanel({ quiet: false, inCenter: true });
  }, [addPlanPanel]);

  return {
    pendingComments,
    commentCount: pendingComments.length,
    subjectEdited: isSubjectEdited(planUpdatedAt, requestCreatedAt),
    openPlan,
  };
}
