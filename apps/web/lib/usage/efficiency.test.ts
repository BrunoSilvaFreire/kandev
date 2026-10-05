import { describe, expect, it } from "vitest";
import type { UsageTotals } from "@/lib/api/domains/usage-api";
import {
  CACHE_EXPIRY_MS,
  HIGH_COST_SUBCENTS_THRESHOLD,
  LOW_HIT_RATIO_MIN_EVENTS,
  cacheStatus,
  estimatedExpiryAt,
  hitRatio,
  usageFlags,
} from "./efficiency";

function totals(overrides: Partial<UsageTotals> = {}): UsageTotals {
  return {
    scope: "session",
    scope_id: "session-1",
    tokens_in: 0,
    tokens_cached_read: 0,
    tokens_cached_write: 0,
    tokens_out: 0,
    tokens_thought: 0,
    tokens_total: 0,
    cost_subcents: 0,
    event_count: 0,
    estimated_event_count: 0,
    unpriced_event_count: 0,
    output_tokens_complete: true,
    first_event_at: null,
    last_event_at: null,
    ...overrides,
  };
}

describe("hitRatio", () => {
  it("is null when the provider reports no cache tokens", () => {
    expect(hitRatio(totals({ tokens_in: 100 }))).toBeNull();
  });

  it("is null when the denominator is zero", () => {
    expect(hitRatio(totals())).toBeNull();
  });

  it("divides cached reads by all input tokens", () => {
    expect(
      hitRatio(totals({ tokens_in: 100, tokens_cached_read: 300, tokens_cached_write: 0 })),
    ).toBe(0.75);
  });
});

describe("cacheStatus", () => {
  const now = 1_000_000;

  it("is unknown without a usage event", () => {
    expect(cacheStatus({ lastUsageEventAt: null, eventCount: 5, now })).toBe("unknown");
  });

  it("is unknown without usage events", () => {
    expect(cacheStatus({ lastUsageEventAt: now, eventCount: 0, now })).toBe("unknown");
  });

  it("is warm inside the expiry window", () => {
    expect(cacheStatus({ lastUsageEventAt: now - CACHE_EXPIRY_MS + 1, eventCount: 2, now })).toBe(
      "warm",
    );
  });

  it("is likely expired at and beyond the expiry window", () => {
    expect(cacheStatus({ lastUsageEventAt: now - CACHE_EXPIRY_MS, eventCount: 2, now })).toBe(
      "likely_expired",
    );
  });

  // A fresh session message must not warm the cache: only a provider round-trip
  // (a usage event) can, so an old event stays likely_expired.
  it("stays likely_expired while the newest usage event is old", () => {
    expect(cacheStatus({ lastUsageEventAt: now - CACHE_EXPIRY_MS - 1, eventCount: 3, now })).toBe(
      "likely_expired",
    );
  });
});

describe("usageFlags", () => {
  it("does not flag a hit ratio exactly at the threshold", () => {
    const flags = usageFlags({
      totals: totals({
        tokens_in: 100,
        tokens_cached_read: 100,
        event_count: LOW_HIT_RATIO_MIN_EVENTS,
      }),
      cacheStatus: "warm",
    });
    expect(flags.lowHitRatio).toBe(false);
  });

  it("flags a hit ratio below the threshold", () => {
    const flags = usageFlags({
      totals: totals({
        tokens_in: 300,
        tokens_cached_read: 100,
        event_count: LOW_HIT_RATIO_MIN_EVENTS,
      }),
      cacheStatus: "warm",
    });
    expect(flags.lowHitRatio).toBe(true);
  });

  it("suppresses a low hit ratio below the minimum event count", () => {
    const flags = usageFlags({
      totals: totals({
        tokens_in: 300,
        tokens_cached_read: 100,
        event_count: LOW_HIT_RATIO_MIN_EVENTS - 1,
      }),
      cacheStatus: "warm",
    });
    expect(flags.lowHitRatio).toBe(false);
  });

  it("does not flag cost below the high-cost boundary", () => {
    const flags = usageFlags({
      totals: totals({ cost_subcents: HIGH_COST_SUBCENTS_THRESHOLD - 1 }),
      cacheStatus: "warm",
    });
    expect(flags.highCost).toBe(false);
  });

  it("flags cost at the high-cost boundary", () => {
    const flags = usageFlags({
      totals: totals({ cost_subcents: HIGH_COST_SUBCENTS_THRESHOLD }),
      cacheStatus: "warm",
    });
    expect(flags.highCost).toBe(true);
  });

  it("flags an expired cache", () => {
    const flags = usageFlags({ totals: totals(), cacheStatus: "likely_expired" });
    expect(flags.cacheLikelyExpired).toBe(true);
  });
});

describe("estimatedExpiryAt", () => {
  it("is null without a usage event", () => {
    expect(estimatedExpiryAt(null)).toBeNull();
  });

  it("adds the expiry window to the newest usage event", () => {
    expect(estimatedExpiryAt(1_000)).toBe(1_000 + CACHE_EXPIRY_MS);
  });
});
