"use client";

import { useCallback, useEffect } from "react";
import { useAppStore } from "@/components/state-provider";
import { useRouter } from "@/lib/routing/client-router";
import { getTaskTransitionFailure } from "@/lib/api/domains/task-activity-api";
import { linkToTask } from "@/lib/links";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { getTransitionFailure, type TransitionFailureRecord } from "@/lib/tasks/transition-failure";

/**
 * The task's current unresolved asynchronous transition failure. Loaded once
 * per task so it survives a reload or reconnect, then kept live by the
 * `task.transition_failure.updated` handler.
 */
export function useTaskTransitionFailure(taskId: string | null): TransitionFailureRecord | null {
  const record = useAppStore((state) =>
    taskId ? (state.taskTransitionFailures[taskId] ?? null) : null,
  );
  const setTaskTransitionFailure = useAppStore((state) => state.setTaskTransitionFailure);
  useEffect(() => {
    if (!taskId) return;
    let cancelled = false;
    getTaskTransitionFailure(taskId)
      .then((loaded) => {
        if (!cancelled) setTaskTransitionFailure(taskId, loaded);
      })
      .catch(() => {
        // An older server has no endpoint; the live event still applies.
      });
    return () => {
      cancelled = true;
    };
  }, [taskId, setTaskTransitionFailure]);
  return record;
}

/** Opens the active task's plan: the dock panel on desktop, the plan route elsewhere. */
export function useOpenTaskPlan(): () => void {
  const taskId = useAppStore((state) => state.tasks.activeTaskId);
  const addPlanPanel = useDockviewStore((state) => state.addPlanPanel);
  const api = useDockviewStore((state) => state.api);
  const router = useRouter();
  return useCallback(() => {
    if (api) {
      addPlanPanel({ quiet: false, inCenter: true });
      return;
    }
    if (taskId) router.push(`${linkToTask(taskId)}?panel=plan`);
  }, [addPlanPanel, api, router, taskId]);
}

/**
 * For move surfaces whose only feedback is a toast: keeps the refusal's
 * reasons reachable in the app-wide notice after the toast expires.
 */
export function useReportTransitionFailure(): (error: unknown) => void {
  const setTransitionFailureNotice = useAppStore((state) => state.setTransitionFailureNotice);
  return useCallback(
    (error: unknown) => {
      const failure = getTransitionFailure(error);
      if (failure) setTransitionFailureNotice(failure);
    },
    [setTransitionFailureNotice],
  );
}
