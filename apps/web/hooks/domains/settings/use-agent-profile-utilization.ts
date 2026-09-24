"use client";

import { useEffect, useMemo, useState } from "react";
import {
  fetchProfileUtilization,
  type ProfileUtilizationItem,
} from "@/lib/api/domains/agent-profile-utilization-api";

export type AgentProfileUtilizationState = {
  items: Record<string, ProfileUtilizationItem>;
  loading: boolean;
  error: unknown;
};

/** The batch endpoint rejects more than 50 unique ids per request. */
export const PROFILE_UTILIZATION_BATCH_SIZE = 50;

export function chunkProfileIds(ids: string[], size = PROFILE_UTILIZATION_BATCH_SIZE): string[][] {
  if (size <= 0) return [ids];
  const batches: string[][] = [];
  for (let i = 0; i < ids.length; i += size) {
    batches.push(ids.slice(i, i + size));
  }
  return batches;
}

/**
 * Loads batch subscription utilization for the given profile IDs, splitting
 * requests at the endpoint's 50-ID limit and merging the results. A failed
 * batch marks only its own candidates unavailable. Fetches only when enabled
 * and at least one ID is requested, so settings boot never calls a provider.
 */
export function useAgentProfileUtilization(
  profileIds: string[],
  enabled: boolean,
): AgentProfileUtilizationState {
  const key = useMemo(() => [...profileIds].sort().join(","), [profileIds]);
  const [items, setItems] = useState<Record<string, ProfileUtilizationItem>>({});
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<unknown>(null);

  useEffect(() => {
    if (!enabled || key === "") {
      setItems({});
      setLoading(false);
      setError(null);
      return;
    }
    const controller = new AbortController();
    setLoading(true);
    setError(null);
    const batches = chunkProfileIds(key.split(","));

    void (async () => {
      const settled = await Promise.allSettled(
        batches.map((batch) => fetchProfileUtilization(batch, { init: { signal: controller.signal } })),
      );
      if (controller.signal.aborted) return;
      const merged: Record<string, ProfileUtilizationItem> = {};
      let failed = false;
      settled.forEach((result, index) => {
        if (result.status === "fulfilled") {
          for (const item of result.value) merged[item.profile_id] = item;
          return;
        }
        failed = true;
        for (const id of batches[index]) {
          merged[id] = { profile_id: id, state: "unavailable" };
        }
      });
      setItems(merged);
      setError(failed ? new Error("profile utilization request failed") : null);
      setLoading(false);
    })();

    return () => {
      controller.abort();
    };
  }, [key, enabled]);

  return { items, loading, error };
}
