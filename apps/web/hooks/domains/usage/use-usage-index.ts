"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { fetchUsageIndexStatus, startUsageIndex } from "@/lib/api/domains/provider-usage-api";
import type { ProviderUsageIndexStatus } from "@/lib/types/provider-usage";

// The index job is a bounded, resumable pass; polling every 2 s only while a
// run is active keeps progress live without a background poller.
const INDEX_POLL_MS = 2_000;

export type UsageIndexController = {
  status: ProviderUsageIndexStatus | null;
  reindex: () => void;
  refresh: () => void;
};

export type UsageIndexOptions = {
  /** Called when a run transitions from running to done or failed. */
  onFinished?: () => void;
};

/**
 * Owns the history index job: it auto-starts a never-run index on mount and
 * polls while the job is running. `reindex` starts an incremental run on
 * demand. `onFinished` lets the page refetch the overview so backfilled
 * history appears as soon as the run ends.
 */
export function useUsageIndex(options?: UsageIndexOptions): UsageIndexController {
  const [status, setStatus] = useState<ProviderUsageIndexStatus | null>(null);
  const onFinishedRef = useRef(options?.onFinished);
  onFinishedRef.current = options?.onFinished;
  const previousStateRef = useRef<string | null>(null);

  useEffect(() => {
    const current = status?.state ?? null;
    if (previousStateRef.current === "running" && (current === "done" || current === "failed")) {
      onFinishedRef.current?.();
    }
    previousStateRef.current = current;
  }, [status]);

  const refresh = useCallback(async () => {
    try {
      setStatus(await fetchUsageIndexStatus());
    } catch {
      // A failed status read is not fatal; the page keeps its last value.
    }
  }, []);

  const reindex = useCallback(async () => {
    try {
      setStatus(await startUsageIndex());
    } catch {
      // A failed start leaves the previous status in place.
    }
  }, []);

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const current = await fetchUsageIndexStatus();
        if (cancelled) return;
        setStatus(current);
        if (current.state === "never") {
          const started = await startUsageIndex();
          if (!cancelled) setStatus(started);
        }
      } catch {
        // Ignore: the Refresh action retries.
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    if (status?.state !== "running") return;
    const timer = window.setTimeout(() => {
      void refresh();
    }, INDEX_POLL_MS);
    return () => window.clearTimeout(timer);
  }, [status, refresh]);

  return { status, reindex, refresh };
}
