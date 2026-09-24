import { describe, expect, it } from "vitest";
import type { UsageGroup, UsageTotals } from "@/lib/api/domains/usage-api";
import { addTotals, groupKey, rollUpGroups, usageRowStats } from "./breakdown";

function totals(overrides: Partial<UsageTotals> = {}): UsageTotals {
  return {
    scope: "group",
    scope_id: "",
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

function group(overrides: Partial<UsageGroup> = {}): UsageGroup {
  return {
    session_id: "session-1",
    agent_profile_id: "profile-1",
    agent_type: "claude",
    model: "model-x",
    provider: "provider-1",
    totals: totals(),
    ...overrides,
  };
}

describe("addTotals", () => {
  it("sums additive fields, ANDs completeness, and takes min/max timestamps", () => {
    const merged = addTotals(
      totals({
        tokens_in: 10,
        cost_subcents: 5,
        event_count: 1,
        output_tokens_complete: true,
        first_event_at: "2026-08-23T04:00:00Z",
        last_event_at: "2026-08-23T04:05:00Z",
      }),
      totals({
        tokens_in: 20,
        cost_subcents: 7,
        event_count: 2,
        output_tokens_complete: false,
        first_event_at: "2026-08-23T03:00:00Z",
        last_event_at: "2026-08-23T06:00:00Z",
      }),
    );

    expect(merged.tokens_in).toBe(30);
    expect(merged.cost_subcents).toBe(12);
    expect(merged.event_count).toBe(3);
    expect(merged.output_tokens_complete).toBe(false);
    expect(merged.first_event_at).toBe("2026-08-23T03:00:00Z");
    expect(merged.last_event_at).toBe("2026-08-23T06:00:00Z");
  });
});

describe("rollUpGroups", () => {
  it("rolls groups up by agent and by model", () => {
    const groups: UsageGroup[] = [
      group({ agent_profile_id: "profile-1", model: "x", totals: totals({ tokens_in: 100, event_count: 1 }) }),
      group({ agent_profile_id: "profile-1", model: "y", totals: totals({ tokens_in: 200, event_count: 1 }) }),
      group({ agent_profile_id: "profile-2", model: "x", totals: totals({ tokens_in: 400, event_count: 1 }) }),
    ];

    const byAgent = rollUpGroups(groups, "agent");
    expect(byAgent).toHaveLength(2);
    const profileOne = byAgent.find((row) => row.key === "profile-1");
    expect(profileOne?.totals.tokens_in).toBe(300);
    expect(profileOne?.models.sort()).toEqual(["x", "y"]);

    const byModel = rollUpGroups(groups, "model");
    expect(byModel).toHaveLength(2);
    const modelX = byModel.find((row) => row.model === "x");
    expect(modelX?.totals.tokens_in).toBe(500);
  });

  it("keeps a deleted-session group as a null session in the session view", () => {
    const groups: UsageGroup[] = [
      group({ session_id: null, totals: totals({ tokens_in: 5 }) }),
      group({ session_id: "session-1", totals: totals({ tokens_in: 7 }) }),
    ];

    const rows = rollUpGroups(groups, "session");

    expect(rows).toHaveLength(2);
    const deleted = rows.find((row) => row.sessionId === null);
    expect(deleted?.totals.tokens_in).toBe(5);
  });

  it("recomputes the ratio from summed tokens rather than averaging", () => {
    // Group A reports 100% caching (400 read, 0 uncached); group B reports no
    // caching at all. Averaging the two reported ratios would give 100%;
    // recomputing from the summed tokens gives 400 / (1200 + 400) = 25%.
    const groups: UsageGroup[] = [
      group({ agent_profile_id: "a", totals: totals({ tokens_in: 0, tokens_cached_read: 400, event_count: 1 }) }),
      group({ agent_profile_id: "a", totals: totals({ tokens_in: 1200, tokens_cached_read: 0, event_count: 1 }) }),
    ];

    const rows = rollUpGroups(groups, "agent");
    expect(rows).toHaveLength(1);
    expect(usageRowStats(rows[0].totals).hitRatio).toBeCloseTo(0.25);
  });

  it("uses the agent group key fallback for an empty profile id", () => {
    expect(groupKey(group({ agent_profile_id: "" }), "agent")).toBe("__unknown__");
  });
});

describe("usageRowStats", () => {
  it("returns null averages (never NaN) for a zero-prompt row", () => {
    const stats = usageRowStats(totals({ event_count: 0 }));
    expect(stats.costPerPrompt).toBeNull();
    expect(stats.avgInputPerPrompt).toBeNull();
    expect(stats.avgOutputPerPrompt).toBeNull();
  });

  it("derives cost and average token use per prompt", () => {
    const stats = usageRowStats(
      totals({
        tokens_in: 100,
        tokens_cached_read: 300,
        tokens_cached_write: 0,
        tokens_out: 40,
        cost_subcents: 200,
        event_count: 2,
      }),
    );
    expect(stats.costPerPrompt).toBe(100);
    expect(stats.avgInputPerPrompt).toBe(200);
    expect(stats.avgOutputPerPrompt).toBe(20);
    expect(stats.hitRatio).toBe(0.75);
  });

  it("reports a null hit ratio when the provider records no caching", () => {
    const stats = usageRowStats(totals({ tokens_in: 100, event_count: 2 }));
    expect(stats.hitRatio).toBeNull();
  });
});
