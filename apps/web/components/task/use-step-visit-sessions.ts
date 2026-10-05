"use client";

import { useCallback } from "react";
import { useTaskTransitionSummary } from "@/hooks/domains/task/use-task-activity";
import { useActivateTaskSession } from "./use-activate-task-session";
import type { StepVisitSession } from "@/lib/api/domains/task-activity-api";

export type StepVisitSessionsState = {
  sessionsByStep: Readonly<Record<string, StepVisitSession[]>>;
  onOpenSession: (sessionId: string) => void;
};

/**
 * Destination sessions per step plus the sanctioned activation callback, shared
 * by the header hover card and the compact disclosure so both read the same
 * bounded transition summary.
 */
export function useStepVisitSessions(taskId?: string | null): StepVisitSessionsState {
  const summary = useTaskTransitionSummary(taskId, Boolean(taskId));
  const activateSession = useActivateTaskSession();
  const onOpenSession = useCallback(
    (sessionId: string) => activateSession(sessionId),
    [activateSession],
  );
  const sessionsByStep: Record<string, StepVisitSession[]> = {};
  for (const [stepId, visit] of Object.entries(summary.visits)) {
    sessionsByStep[stepId] = visit.sessions;
  }
  return { sessionsByStep, onOpenSession };
}
