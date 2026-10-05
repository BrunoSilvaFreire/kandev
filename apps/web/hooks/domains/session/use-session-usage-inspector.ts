import { useCallback, useEffect, useRef, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import {
  getSessionUsageTotals,
  getTaskUsageTotals,
  type UsageTotals,
} from "@/lib/api/domains/usage-api";
import {
  CACHE_RECHECK_MS,
  cacheStatus,
  epochMillisFromWire,
  estimatedExpiryAt,
  usageFlags,
  type CacheStatus,
  type UsageFlags,
} from "@/lib/usage/efficiency";
import type { PromptUsageEntry } from "@/lib/state/slices/session-runtime/types";
import { t } from "@/lib/i18n";

export type SessionUsageInspector = {
  /** Session-scoped ledger totals (one session is one agent profile). */
  session: UsageTotals | null;
  /** Task-wide ledger totals across every agent in the task. */
  task: UsageTotals | null;
  /** Live last-prompt usage from the store, not the persisted ledger. */
  lastPrompt: PromptUsageEntry | undefined;
  status: CacheStatus;
  /** Estimated instant the prompt cache expires, or null when unknown. */
  expiresAt: number | null;
  flags: UsageFlags | null;
  loading: boolean;
  error: string | null;
};

/**
 * Reads the existing task-cost-ledger routes for the session and task and
 * derives cache status and flags. It fetches only while `enabled`, refetches
 * when the live prompt usage changes, and discards responses that arrive after
 * a session switch.
 */
export function useSessionUsageInspector(
  taskId: string | null,
  sessionId: string | null,
  options: { enabled: boolean },
): SessionUsageInspector {
  const { enabled } = options;
  const [session, setSession] = useState<UsageTotals | null>(null);
  const [task, setTask] = useState<UsageTotals | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [now, setNow] = useState(() => Date.now());
  const generationRef = useRef(0);

  // Cache status must decay to "likely expired" while the surface stays idle.
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), CACHE_RECHECK_MS);
    return () => clearInterval(timer);
  }, []);

  const lastPrompt = useAppStore((state) =>
    sessionId ? state.promptUsage.bySessionId[sessionId] : undefined,
  );

  // Drop the previous scope's totals so a switch cannot briefly show them, and
  // invalidate any in-flight request so it cannot land on the new scope.
  useEffect(() => {
    generationRef.current += 1;
    setSession(null);
    setTask(null);
    setError(null);
  }, [taskId, sessionId, enabled]);

  const load = useCallback(async () => {
    if (!taskId || !sessionId) return;
    const generation = generationRef.current + 1;
    generationRef.current = generation;
    setLoading(true);
    setError(null);
    try {
      const [sessionTotals, taskTotals] = await Promise.all([
        getSessionUsageTotals(taskId, sessionId),
        getTaskUsageTotals(taskId),
      ]);
      if (generation !== generationRef.current) return;
      setSession(sessionTotals);
      setTask(taskTotals);
    } catch (err) {
      if (generation !== generationRef.current) return;
      // i18n-exempt: operator diagnostic; the UI shows translated copy.
      console.error("Failed to load session usage totals", err);
      setError(t("task:usageInspectorLoadFailed"));
    } finally {
      if (generation === generationRef.current) setLoading(false);
    }
  }, [taskId, sessionId]);

  useEffect(() => {
    if (!enabled || !taskId || !sessionId) return;
    void load();
    // `lastPrompt` is a dependency so a new prompt refetches the ledger.
  }, [enabled, taskId, sessionId, lastPrompt, load]);

  const lastUsageEventAt = epochMillisFromWire(session?.last_event_at);
  const status = cacheStatus({
    lastUsageEventAt,
    eventCount: session?.event_count ?? 0,
    now,
  });
  const flags = session ? usageFlags({ totals: session, cacheStatus: status }) : null;

  return {
    session,
    task,
    lastPrompt,
    status,
    expiresAt: estimatedExpiryAt(lastUsageEventAt),
    flags,
    loading,
    error,
  };
}
