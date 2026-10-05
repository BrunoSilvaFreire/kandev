"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { fetchUsageBreakdown } from "@/lib/api/domains/provider-usage-api";
import type {
  ProviderUsageRange,
  UsageBreakdownQuery,
  UsageBreakdownResponse,
} from "@/lib/types/provider-usage";

/** Free-text search is debounced; every other change fetches immediately. */
export const USAGE_BREAKDOWN_DEBOUNCE_MS = 300;

export type UsageBreakdownController = {
  data: UsageBreakdownResponse | null;
  loading: boolean;
  error: boolean;
  query: UsageBreakdownQuery;
  setQuery: (patch: Partial<UsageBreakdownQuery>) => void;
  refresh: () => void;
};

function initialQuery(base?: Partial<UsageBreakdownQuery>): UsageBreakdownQuery {
  return { groupBy: "session", sort: "tokens", order: "desc", offset: 0, ...base };
}

/**
 * Owns the Breakdown tab query state. Filter, group, and sort changes reset
 * pagination; a stale response is discarded by request id so an out-of-order
 * reply cannot overwrite the current view.
 */
export function useUsageBreakdown(
  range: ProviderUsageRange,
  base?: Partial<UsageBreakdownQuery>,
  externalRefreshToken = 0,
): UsageBreakdownController {
  const [query, setQueryState] = useState<UsageBreakdownQuery>(() => initialQuery(base));
  const [debouncedSearch, setDebouncedSearch] = useState(query.q ?? "");
  const [data, setData] = useState<UsageBreakdownResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);
  const [refreshToken, setRefreshToken] = useState(0);
  const requestId = useRef(0);

  useEffect(() => {
    const handle = setTimeout(() => setDebouncedSearch(query.q ?? ""), USAGE_BREAKDOWN_DEBOUNCE_MS);
    return () => clearTimeout(handle);
  }, [query.q]);

  // Built from fields rather than the query object so a keystroke that only
  // changes `q` does not trigger a fetch before the debounce settles.
  const effective = useMemo(
    () => ({
      range,
      groupBy: query.groupBy,
      provider: query.provider,
      model: query.model,
      agentType: query.agentType,
      taskId: query.taskId,
      sessionId: query.sessionId,
      sort: query.sort,
      order: query.order,
      limit: query.limit,
      offset: query.offset,
      q: debouncedSearch || undefined,
    }),
    [
      range,
      query.groupBy,
      query.provider,
      query.model,
      query.agentType,
      query.taskId,
      query.sessionId,
      query.sort,
      query.order,
      query.limit,
      query.offset,
      debouncedSearch,
    ],
  );

  const setQuery = useCallback((patch: Partial<UsageBreakdownQuery>) => {
    setQueryState((previous) => {
      const next = { ...previous, ...patch };
      if (Object.keys(patch).some((key) => key !== "offset")) next.offset = 0;
      return next;
    });
  }, []);

  const refresh = useCallback(() => setRefreshToken((token) => token + 1), []);

  useEffect(() => {
    const id = requestId.current + 1;
    requestId.current = id;
    setLoading(true);
    fetchUsageBreakdown(effective)
      .then((result) => {
        if (id !== requestId.current) return;
        setData(result);
        setError(false);
      })
      .catch(() => {
        if (id !== requestId.current) return;
        setError(true);
      })
      .finally(() => {
        if (id !== requestId.current) return;
        setLoading(false);
      });
  }, [effective, refreshToken, externalRefreshToken]);

  return { data, loading, error, query, setQuery, refresh };
}
