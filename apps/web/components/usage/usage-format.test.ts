import { describe, expect, it } from "vitest";
import type { TFunction } from "i18next";
import {
  estimateChips,
  fetchedChip,
  lastFetchedAt,
  quotaUnavailableReasonKey,
  resetChip,
  windowSourceBadge,
} from "./usage-format";

// The chips only assert key names; echoing the key keeps the tests locale-free.
const t = ((key: string) => key) as unknown as TFunction;

describe("windowSourceBadge", () => {
  it("maps a measured window to reported", () => {
    expect(windowSourceBadge("measured")).toBe("reported");
  });

  it("maps an estimated window to estimated", () => {
    expect(windowSourceBadge("estimated")).toBe("estimated");
  });
});

describe("quotaUnavailableReasonKey", () => {
  it("maps every closed-set reason to a distinct key", () => {
    const keys = [
      quotaUnavailableReasonKey("api_key_billing"),
      quotaUnavailableReasonKey("no_provider_endpoint"),
      quotaUnavailableReasonKey("profile_missing"),
    ];
    expect(new Set(keys).size).toBe(3);
  });
});

describe("lastFetchedAt", () => {
  it("returns the newest valid point only", () => {
    const at = lastFetchedAt([
      { at: "2026-09-25T10:00:00Z", pct: 1, kind: "measured" },
      { at: "2026-09-25T12:00:00Z", pct: 2, kind: "measured" },
      { at: "not-a-timestamp", pct: 3, kind: "measured" },
    ]);
    expect(at).not.toBeNull();
  });

  it("returns null for an empty series", () => {
    expect(lastFetchedAt([])).toBeNull();
  });
});

describe("estimateChips", () => {
  it("replaces the estimate with one chip when confidence is none", () => {
    const chips = estimateChips(t, undefined);
    expect(chips).toHaveLength(1);
    expect(chips[0]?.label).toBe("usage:estimateNotEnough");
    expect(chips[0]?.tone).toBe("neutral");
  });

  it("renders burn, trend, and exhaustion chips", () => {
    const chips = estimateChips(t, {
      burn_rate_pct_per_hour: 31.77,
      projected_exhaustion_at: "2026-09-29T00:00:00Z",
      exhausts_before_reset: true,
      trend: "rising",
      anomaly: false,
      confidence: "medium",
      source: "estimated",
      reset_at: "2026-09-30T00:00:00Z",
    });
    expect(chips.map((chip) => chip.key)).toEqual(["burn", "trend", "exhaustion"]);
    expect(chips[0]?.label).toBe("usage:estimateBurnRate");
    expect(chips[2]?.tone).toBe("exhausted");
  });

  it("renders a resets-first chip when the window resets first", () => {
    const chips = estimateChips(t, {
      burn_rate_pct_per_hour: 1,
      exhausts_before_reset: false,
      trend: "flat",
      anomaly: false,
      confidence: "low",
      source: "estimated",
    });
    expect(chips.at(-1)?.key).toBe("resets");
    expect(chips.at(-1)?.tone).toBe("healthy");
  });
});

describe("resetChip", () => {
  it("explains the exact reset time", () => {
    expect(resetChip(t, undefined).tooltip).toBe("usage:resetTooltipNone");
    expect(resetChip(t, "2026-09-30T00:00:00Z").tooltip).toBe("usage:resetTooltip");
  });
});

describe("fetchedChip", () => {
  it("returns null with no parseable point", () => {
    expect(fetchedChip(t, [], "measured")).toBeNull();
  });

  it("carries provenance in the tooltip", () => {
    const chip = fetchedChip(
      t,
      [{ at: "2026-09-25T10:00:00Z", pct: 1, kind: "measured" }],
      "measured",
    );
    expect(chip?.tooltip).toBe("usage:fetchedTooltip");
  });
});
