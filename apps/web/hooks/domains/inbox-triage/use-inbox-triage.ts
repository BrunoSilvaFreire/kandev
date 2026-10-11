"use client";

import { useCallback, useEffect, useMemo, useState, useSyncExternalStore } from "react";
import { useAppStore } from "@/components/state-provider";
import { selectNeedsYouInboxBundles } from "@/lib/state/slices/needs-you-inbox/selectors";
import {
  acknowledgeTriageItems,
  getAcknowledgedTriageIds,
  getServerAcknowledgedTriageIds,
  pruneAcknowledgedTriage,
  subscribeAcknowledgedTriage,
} from "@/lib/inbox-triage/acknowledged-store";
import {
  NUDGE_KINDS,
  buildInboxTriage,
  selectUnacknowledgedTriageIds,
  type InboxTriageItem,
  type TriageBundleInput,
  type TriageTaskInput,
} from "@/lib/inbox-triage/possible-question-triage";
import type { TaskState } from "@/lib/types/http";

const EMPTY_TASKS: never[] = [];
const EMPTY_SNAPSHOTS: Record<string, { tasks?: unknown[] }> = {};
const EMPTY_BUNDLES: never[] = [];

/** How often a visible tab re-evaluates Review age without a store event. */
export const TRIAGE_CLOCK_MS = 60_000;

/** Coarse clock that ticks only while the tab is visible. */
function useTriageClock(): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const tick = () => {
      if (!document.hidden) setNow(Date.now());
    };
    const timer = window.setInterval(tick, TRIAGE_CLOCK_MS);
    document.addEventListener("visibilitychange", tick);
    return () => {
      window.clearInterval(timer);
      document.removeEventListener("visibilitychange", tick);
    };
  }, []);
  return now;
}

/** Structural view of the kanban task projection the lane reads. */
type KanbanTriageTask = {
  id: string;
  title: string;
  state?: string;
  primarySessionId?: string | null;
  interrupted?: boolean | null;
  interruption?: TriageTaskInput["interruption"];
  reviewEpisodeId?: string | null;
  reviewEnteredAt?: string | null;
  blocked?: boolean | null;
  metadata?: Record<string, unknown> | null;
  updatedAt?: string | null;
  statusSummary?: {
    possible_question?: boolean;
    possible_question_turn_id?: string;
  } | null;
};

function toTriageTask(task: KanbanTriageTask): TriageTaskInput {
  const interruptedAt = task.metadata?.interrupted_at;
  return {
    id: task.id,
    title: task.title,
    state: task.state as TaskState | undefined,
    primary_session_id: task.primarySessionId ?? null,
    possible_question: task.statusSummary?.possible_question ?? false,
    possible_question_turn_id: task.statusSummary?.possible_question_turn_id ?? null,
    // Unset stays unset: only an explicit false retires a cached episode.
    interrupted: task.interrupted,
    interrupted_at: typeof interruptedAt === "string" ? interruptedAt : null,
    interruption: task.interruption ?? null,
    review_episode_id: task.reviewEpisodeId ?? null,
    review_entered_at: task.reviewEnteredAt ?? null,
    blocked: task.blocked ?? false,
    updated_at: task.updatedAt ?? null,
  };
}

/** Builds the ordered triage lane from the current store projections. */
export function useInboxTriageItems(): InboxTriageItem[] {
  const kanbanTasks = useAppStore((s) => s.kanban?.tasks ?? EMPTY_TASKS);
  const snapshots = useAppStore((s) => s.kanbanMulti?.snapshots ?? EMPTY_SNAPSHOTS);
  const bundles = useAppStore(selectNeedsYouInboxBundles);
  const now = useTriageClock();
  return useMemo(() => {
    const tasks: KanbanTriageTask[] = [...(kanbanTasks as KanbanTriageTask[])];
    for (const snapshot of Object.values(snapshots)) {
      tasks.push(...((snapshot.tasks ?? []) as KanbanTriageTask[]));
    }
    return buildInboxTriage(
      tasks.map(toTriageTask),
      (bundles ?? EMPTY_BUNDLES) as readonly TriageBundleInput[],
      now,
    );
  }, [kanbanTasks, snapshots, bundles, now]);
}

export type InboxTriageNudge = {
  items: InboxTriageItem[];
  /** Unacknowledged identities across all kinds. */
  newCount: number;
  /**
   * Unacknowledged identities the clarification-backed inbox does not already
   * count: interrupted work, stale reviews, and possible questions.
   * Navigation badges add this to
   * the clarification count so a lane-only item is visible off the inbox page.
   */
  additionalCount: number;
  acknowledgeAll: () => void;
};

/**
 * Derives the in-app nudge count from the triage lane and deduplicates it by
 * surfaced identity (task/session/turn) in a shared store. It never dispatches
 * or suppresses a provider notification; viewing the lane acknowledges the
 * current identities.
 */
export function useInboxTriageNudge(): InboxTriageNudge {
  const items = useInboxTriageItems();
  const acknowledged = useSyncExternalStore(
    subscribeAcknowledgedTriage,
    getAcknowledgedTriageIds,
    getServerAcknowledgedTriageIds,
  );

  // Keep storage bounded: drop acknowledgements for identities no longer
  // surfaced. Skipped while the lane is empty so an unhydrated store does not
  // wipe acknowledgements the operator already made.
  // TODO: prune only after every workspace snapshot has hydrated; a partially loaded store drops acks for unloaded items and re-nudges them.
  useEffect(() => {
    if (items.length === 0) return;
    pruneAcknowledgedTriage(new Set(items.map((item) => item.id)));
  }, [items]);

  const acknowledgeAll = useCallback(() => {
    acknowledgeTriageItems(items.map((item) => item.id));
  }, [items]);

  const newCount = useMemo(
    () => selectUnacknowledgedTriageIds(items, acknowledged).length,
    [items, acknowledged],
  );
  const additionalCount = useMemo(
    () =>
      items.filter(
        (item) =>
          item.kind !== "clarification" && NUDGE_KINDS.has(item.kind) && !acknowledged.has(item.id),
      ).length,
    [items, acknowledged],
  );

  return { items, newCount, additionalCount, acknowledgeAll };
}
