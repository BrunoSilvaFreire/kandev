"use client";

import { useCallback } from "react";
import { useTranslation } from "react-i18next";
import { useToast } from "@/components/toast-provider";
import { getWebSocketClient } from "@/lib/ws/connection";
import { moveTask } from "@/lib/api";

export type HandoffStepMove = {
  /**
   * Promotes the new handoff session to primary so the subsequent move routes
   * to it. Returns false (and toasts) when promotion fails, which skips the
   * move; the session is kept either way.
   */
  promotePrimary: (sessionId: string) => Promise<boolean>;
  /** Moves the task through the normal move API; toasts on failure. */
  moveToStep: (
    taskId: string,
    sessionId: string,
    workflowId: string,
    targetStepId: string,
  ) => Promise<void>;
};

/**
 * Applies the optional step transition for a handoff. The order is fixed:
 * promote the session to primary, activate it locally (in the launch submit),
 * then move the task. A failed promotion leaves the session in place and skips
 * the move; a failed move keeps the session. Both surface a toast and never
 * throw.
 */
export function useHandoffStepMove(): HandoffStepMove {
  const { t } = useTranslation();
  const { toast } = useToast();

  const promotePrimary = useCallback(
    async (sessionId: string) => {
      const client = getWebSocketClient();
      try {
        if (!client) throw new Error("WebSocket client not available");
        await client.request("session.set_primary", { session_id: sessionId }, 10_000);
        return true;
      } catch {
        toast({ title: t("task:handoffPrimaryFailed"), variant: "error" });
        return false;
      }
    },
    [t, toast],
  );

  const moveToStep = useCallback(
    async (taskId: string, _sessionId: string, workflowId: string, targetStepId: string) => {
      try {
        await moveTask(taskId, { workflow_id: workflowId, workflow_step_id: targetStepId });
      } catch {
        toast({ title: t("task:handoffMoveFailed"), variant: "error" });
      }
    },
    [t, toast],
  );

  return { promotePrimary, moveToStep };
}
