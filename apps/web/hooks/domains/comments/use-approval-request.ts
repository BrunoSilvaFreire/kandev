"use client";

import { useCallback } from "react";
import { useAppStore } from "@/components/state-provider";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { useRouter } from "@/lib/routing/client-router";
import { linkToTask } from "@/lib/links";
import { isSubjectEdited } from "@/lib/approval";
import { usePendingPlanComments } from "./use-pending-comments";

/**
 * useApprovalRequest supplies the live data an approval card needs: the task's
 * pending plan comments, whether the plan changed since the request, and an
 * Open-plan action. Task-scoped reads go through the active task.
 *
 * Open plan prefers the live dockview. The Quick Chat panel has no dockview, so
 * there it navigates to the Quick Chat detail page with the plan panel
 * requested; the detail page owns a dockview and opens it.
 */
export function useApprovalRequest(requestCreatedAt: string | null | undefined) {
  const taskId = useAppStore((s) => s.tasks.activeTaskId);
  const planUpdatedAt = useAppStore((s) =>
    taskId ? (s.taskPlans.byTaskId[taskId]?.updated_at ?? null) : null,
  );
  const pendingComments = usePendingPlanComments(taskId);
  const addPlanPanel = useDockviewStore((s) => s.addPlanPanel);
  const api = useDockviewStore((s) => s.api);
  const isQuickChatTask = useAppStore((s) =>
    s.quickChat.sessions.some((session) => session.taskId === taskId),
  );
  const router = useRouter();

  const openPlan = useCallback(() => {
    if (api) {
      addPlanPanel({ quiet: false, inCenter: true });
      return;
    }
    if (!taskId) return;
    if (isQuickChatTask) router.push(`/quick-chats/${taskId}?panel=plan`);
    else router.push(`${linkToTask(taskId)}?panel=plan`);
  }, [addPlanPanel, api, isQuickChatTask, router, taskId]);

  return {
    pendingComments,
    commentCount: pendingComments.length,
    subjectEdited: isSubjectEdited(planUpdatedAt, requestCreatedAt),
    openPlan,
  };
}
