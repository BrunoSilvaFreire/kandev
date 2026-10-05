"use client";

import { useCallback, useEffect, useState } from "react";
import { fetchProviderUsage } from "@/lib/api/domains/provider-usage-api";
import type { ProviderUsageOverview, ProviderUsageRange } from "@/lib/types/provider-usage";

export type ProviderUsageController = {
  overview: ProviderUsageOverview | null;
  loading: boolean;
  error: boolean;
  refresh: () => void;
};

/**
 * Reads the provider usage overview on mount, on range change, and on an
 * explicit Refresh. The page is the only consumer; there is no polling.
 */
export function useProviderUsage(
  range: ProviderUsageRange,
  refreshToken = 0,
): ProviderUsageController {
  const [overview, setOverview] = useState<ProviderUsageOverview | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const result = await fetchProviderUsage(range);
      setOverview(result);
      setError(false);
    } catch {
      setError(true);
    } finally {
      setLoading(false);
    }
  }, [range, refreshToken]);

  useEffect(() => {
    void load();
  }, [load]);

  return { overview, loading, error, refresh: load };
}
