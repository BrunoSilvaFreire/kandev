"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import {
  getTaskTransitionSummary,
  listTaskActivity,
  type StepVisitSession,
  type TaskActivityEvent,
  type TaskStepTransition,
} from "@/lib/api/domains/task-activity-api";

export type ActivityLoadStatus = "idle" | "loading" | "success" | "error";

export type StepVisitState = {
  count: number;
  /** Persisted destination sessions ordered oldest to newest, deduped. */
  sessions: StepVisitSession[];
};

export type TransitionSummaryState = {
  /** Committed count per directed `from->to` pair. */
  counts: Record<string, number>;
  /** Visit (entry) count per destination step plus its destination sessions. */
  visits: Record<string, StepVisitState>;
  status: ActivityLoadStatus;
  reload: () => void;
};

const EMPTY_SUMMARY: Omit<TransitionSummaryState, "reload"> = {
  counts: {},
  visits: {},
  status: "idle",
};

/** Bounded header summary: committed directed counts and per-step visits. */
export function useTaskTransitionSummary(
  taskId: string | null | undefined,
  enabled = true,
): TransitionSummaryState {
  const connectionStatus = useAppStore((state) => state.connection.status);
  const [state, setState] = useState(EMPTY_SUMMARY);
  const [reloadToken, setReloadToken] = useState(0);
  const requestRef = useRef(0);

  useEffect(() => {
    const requestId = ++requestRef.current;
    if (!enabled || !taskId) {
      setState(EMPTY_SUMMARY);
      return;
    }
    setState((current) => ({ ...current, status: "loading" }));
    getTaskTransitionSummary(taskId)
      .then((summary) => {
        if (requestRef.current !== requestId) return;
        const counts: Record<string, number> = {};
        for (const entry of summary.counts)
          counts[`${entry.from_step_id}->${entry.to_step_id}`] = entry.count;
        const visits: TransitionSummaryState["visits"] = {};
        for (const visit of summary.visits) {
          visits[visit.step_id] = {
            count: visit.count,
            sessions: visit.sessions ?? [],
          };
        }
        setState({ counts, visits, status: "success" });
      })
      .catch(() => {
        if (requestRef.current !== requestId) return;
        setState({ ...EMPTY_SUMMARY, status: "error" });
      });
    return () => {
      if (requestRef.current === requestId) requestRef.current += 1;
    };
  }, [connectionStatus, enabled, reloadToken, taskId]);

  const reload = useCallback(() => setReloadToken((token) => token + 1), []);
  return { ...state, reload };
}

export type TaskActivityState = {
  events: TaskActivityEvent[];
  status: ActivityLoadStatus;
  hasMore: boolean;
  loadMore: () => void;
  reload: () => void;
};

const EMPTY_ACTIVITY: Omit<TaskActivityState, "loadMore" | "reload"> = {
  events: [],
  status: "idle",
  hasMore: false,
};

/**
 * Cursor-paginated task activity, optionally narrowed to one destination step.
 * Loaded on demand; never eager-loads the full history.
 */
export function useTaskActivity(
  taskId: string | null | undefined,
  options: { stepId?: string | null; enabled?: boolean } = {},
): TaskActivityState {
  const { stepId = null, enabled = true } = options;
  const connectionStatus = useAppStore((state) => state.connection.status);
  const [state, setState] = useState(EMPTY_ACTIVITY);
  const [reloadToken, setReloadToken] = useState(0);
  const requestRef = useRef(0);
  const cursorRef = useRef<string | undefined>(undefined);
  const loadingRef = useRef(false);

  useEffect(() => {
    const requestId = ++requestRef.current;
    cursorRef.current = undefined;
    if (!enabled || !taskId) {
      setState(EMPTY_ACTIVITY);
      return;
    }
    setState({ events: [], status: "loading", hasMore: false });
    listTaskActivity(taskId, { stepId: stepId ?? undefined })
      .then((page) => {
        if (requestRef.current !== requestId) return;
        cursorRef.current = page.next_cursor;
        setState({ events: page.events, status: "success", hasMore: page.has_more });
      })
      .catch(() => {
        if (requestRef.current !== requestId) return;
        setState({ events: [], status: "error", hasMore: false });
      });
    return () => {
      if (requestRef.current === requestId) requestRef.current += 1;
    };
  }, [connectionStatus, enabled, reloadToken, stepId, taskId]);

  const loadMore = useCallback(() => {
    if (!taskId || !cursorRef.current || loadingRef.current) return;
    const requestId = requestRef.current;
    loadingRef.current = true;
    listTaskActivity(taskId, { stepId: stepId ?? undefined, cursor: cursorRef.current })
      .then((page) => {
        if (requestRef.current !== requestId) return;
        cursorRef.current = page.next_cursor;
        setState((current) => ({
          events: current.events.concat(page.events),
          status: "success",
          hasMore: page.has_more,
        }));
      })
      .catch(() => {
        loadingRef.current = false;
      })
      .finally(() => {
        loadingRef.current = false;
      });
  }, [stepId, taskId]);

  const reload = useCallback(() => setReloadToken((token) => token + 1), []);
  return { ...state, loadMore, reload };
}

export type StepVisitsState = {
  visits: TaskActivityEvent[];
  status: ActivityLoadStatus;
  hasMore: boolean;
  loadMore: () => void;
  reload: () => void;
};

/** Chronological step visits, loaded on demand. Thin alias over useTaskActivity. */
export function useStepVisits(
  taskId: string | null | undefined,
  stepId: string | null,
  enabled = true,
): StepVisitsState {
  const state = useTaskActivity(taskId, { stepId, enabled: enabled && Boolean(stepId) });
  return {
    visits: state.events,
    status: state.status,
    hasMore: state.hasMore,
    loadMore: state.loadMore,
    reload: state.reload,
  };
}

/** Narrow a visit event to its transition payload for display. */
export function visitTransition(event: TaskActivityEvent): TaskStepTransition | null {
  return event.kind === "transition" ? (event.transition ?? null) : null;
}
