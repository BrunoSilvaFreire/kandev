import type { UsageTotals } from "@/lib/api/domains/usage-api";

/**
 * Estimated prompt-cache lifetime. Providers do not expose a TTL, and observed
 * lifetimes range from minutes to an hour, so this is a heuristic, not a fact.
 * Shared with the resume-with-handoff offer so both use one definition.
 */
export const CACHE_EXPIRY_MS = 60 * 60 * 1000;

/** How often cache status is recomputed while a surface stays mounted. */
export const CACHE_RECHECK_MS = 60_000;

/** Below this hit ratio (once enough prompts have run) the cache is under-used. */
export const LOW_HIT_RATIO_THRESHOLD = 0.5;

/** Minimum usage events before a low hit ratio is worth flagging. */
export const LOW_HIT_RATIO_MIN_EVENTS = 3;

/** Session cost (subcents) at or above which a session is flagged as high ($5). */
export const HIGH_COST_SUBCENTS_THRESHOLD = 50_000;

export type CacheStatus = "warm" | "likely_expired" | "unknown";

export type CacheStatusInput = {
  newestMessageAt: number | null;
  eventCount: number;
  now: number;
};

/**
 * Prompt-cache hit ratio: cached reads over all input tokens (uncached input
 * plus cache reads and writes). Null when no caching is reported at all, so the
 * UI can say "not reported" instead of showing a misleading zero.
 */
export function hitRatio(totals: UsageTotals): number | null {
  const cacheTokens = totals.tokens_cached_read + totals.tokens_cached_write;
  if (cacheTokens === 0) return null;
  const denominator = totals.tokens_in + cacheTokens;
  if (denominator === 0) return null;
  return totals.tokens_cached_read / denominator;
}

/** Cache state from the shared expiry heuristic. */
export function cacheStatus(input: CacheStatusInput): CacheStatus {
  if (input.eventCount <= 0 || input.newestMessageAt === null) return "unknown";
  return input.now - input.newestMessageAt < CACHE_EXPIRY_MS ? "warm" : "likely_expired";
}

export type UsageFlags = {
  lowHitRatio: boolean;
  highCost: boolean;
  cacheLikelyExpired: boolean;
};

export function usageFlags(input: { totals: UsageTotals; cacheStatus: CacheStatus }): UsageFlags {
  const ratio = hitRatio(input.totals);
  return {
    lowHitRatio:
      ratio !== null &&
      ratio < LOW_HIT_RATIO_THRESHOLD &&
      input.totals.event_count >= LOW_HIT_RATIO_MIN_EVENTS,
    highCost: input.totals.cost_subcents >= HIGH_COST_SUBCENTS_THRESHOLD,
    cacheLikelyExpired: input.cacheStatus === "likely_expired",
  };
}

/** Estimated instant the prompt cache expires for the newest message. */
export function estimatedExpiryAt(newestMessageAt: number | null): number | null {
  return newestMessageAt === null ? null : newestMessageAt + CACHE_EXPIRY_MS;
}
