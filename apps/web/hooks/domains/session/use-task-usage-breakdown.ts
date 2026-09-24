import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import {
  getTaskUsageBreakdown,
  type UsageBreakdown,
  type UsageGroup,
  type UsageTotals,
} from "@/lib/api/domains/usage-api";
import { CACHE_RECHECK_MS } from "@/lib/usage/efficiency";
import type { PromptUsageEntry } from "@/lib/state/slices/session-runtime/types";
import { t } from "@/lib/i18n";

const EMPTY_GROUPS: UsageGroup[] = [];

export type TaskUsageBreakdown = {
  task: UsageTotals | null;
  groups: UsageGroup[];
  /** Live last-prompt usage for this task's sessions only. */
  lastPromptBySession: Record<string, PromptUsageEntry>;
  loading: boolean;
  error: string | null;
  /** Recomputes on the shared cache tick so cache status can decay while idle. */
  now: number;
};

function promptUsageSignature(entries: Record<string, PromptUsageEntry>): string {
  return Object.keys(entries)
    .sort()
    .map((id) => {
      const entry = entries[id];
      return [
        id,
        entry.inputTokens,
        entry.outputTokens,
        entry.cachedReadTokens ?? 0,
        entry.cachedWriteTokens ?? 0,
        entry.totalTokens,
      ].join(":");
    })
    .join("|");
}

/**
 * Reads the task-cost-ledger breakdown for one task. It fetches on mount and
 * refetches only when a prompt finishes in one of this task's sessions (other
 * tasks' prompts do not refetch it); a shared one-minute tick advances `now`
 * so cache status can decay without a fetch. Responses landing after a task
 * switch or disable are discarded.
 */
export function useTaskUsageBreakdown(
  taskId: string | null,
  options: { enabled: boolean },
): TaskUsageBreakdown {
  const { enabled } = options;
  const [breakdown, setBreakdown] = useState<UsageBreakdown | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [now, setNow] = useState(() => Date.now());
  const generationRef = useRef(0);

  const sessions = useAppStore((state) =>
    taskId ? state.taskSessionsByTask.itemsByTaskId[taskId] : undefined,
  );
  const promptUsageBySession = useAppStore((state) => state.promptUsage.bySessionId);

  const lastPromptBySession = useMemo(() => {
    const out: Record<string, PromptUsageEntry> = {};
    for (const session of sessions ?? []) {
      const entry = promptUsageBySession[session.id];
      if (entry) out[session.id] = entry;
    }
    return out;
  }, [sessions, promptUsageBySession]);

  const promptUsageKey = useMemo(
    () => promptUsageSignature(lastPromptBySession),
    [lastPromptBySession],
  );

  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), CACHE_RECHECK_MS);
    return () => clearInterval(timer);
  }, []);

  // Drop the previous task's data and invalidate any in-flight request so it
  // cannot land after a switch or a disable.
  useEffect(() => {
    generationRef.current += 1;
    setBreakdown(null);
    setError(null);
  }, [taskId, enabled]);

  const load = useCallback(async () => {
    if (!taskId) return;
    const generation = generationRef.current + 1;
    generationRef.current = generation;
    setLoading(true);
    setError(null);
    try {
      const result = await getTaskUsageBreakdown(taskId);
      if (generation !== generationRef.current) return;
      setBreakdown(result);
    } catch (err) {
      if (generation !== generationRef.current) return;
      // i18n-exempt: operator diagnostic; the UI shows translated copy.
      console.error("Failed to load task usage breakdown", err);
      setError(t("task:usagePanelLoadFailed"));
    } finally {
      if (generation === generationRef.current) setLoading(false);
    }
  }, [taskId]);

  useEffect(() => {
    if (!enabled || !taskId) return;
    void load();
  }, [enabled, taskId, promptUsageKey, load]);

  return {
    task: breakdown?.task ?? null,
    groups: breakdown?.groups ?? EMPTY_GROUPS,
    lastPromptBySession,
    loading,
    error,
    now,
  };
}
