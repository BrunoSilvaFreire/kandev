import { describe, expect, it } from "vitest";
import type { TFunction } from "i18next";
import {
  formatBalance,
  subscriptionPeriodChip,
  subscriptionStatusKey,
  subscriptionStatusTone,
} from "./usage-facts-format";

const t = ((key: string, options?: Record<string, unknown>) => {
  if (options && "amount" in options) return `${key}:${options.amount}`;
  if (options && "date" in options) return `${key}:${options.date}`;
  return key;
}) as unknown as TFunction;

describe("formatBalance", () => {
  it("formats USD as currency", () => {
    expect(formatBalance(t, { label: "prepaid", amount: 6.1, unit: "USD" })).toContain("6.1");
  });

  it("formats credits through the translated template", () => {
    expect(formatBalance(t, { label: "aip", amount: 1234, unit: "CREDITS" })).toContain(
      "usage:balanceUnitCredits",
    );
  });

  it("falls back to the raw unit", () => {
    expect(formatBalance(t, { label: "x", amount: 5, unit: "TOKENS" })).toBe("5 TOKENS");
  });
});

describe("subscriptionStatusKey", () => {
  it("maps every status to a distinct key", () => {
    const keys = (["active", "canceling", "renewal_pending", "inactive"] as const).map(
      subscriptionStatusKey,
    );
    expect(new Set(keys).size).toBe(4);
  });
});

describe("subscriptionStatusTone", () => {
  it("marks active healthy and inactive neutral", () => {
    expect(subscriptionStatusTone("active")).toBe("healthy");
    expect(subscriptionStatusTone("canceling")).toBe("nearing");
    expect(subscriptionStatusTone("inactive")).toBe("neutral");
  });
});

describe("subscriptionPeriodChip", () => {
  it("uses the ends label for a canceling subscription", () => {
    const chip = subscriptionPeriodChip(t, {
      status: "canceling",
      period_ends_at: "2026-10-08T19:29:15.000Z",
      uses_balance: false,
    });
    expect(chip?.label).toContain("usage:subscriptionEnds");
  });

  it("uses the renews label otherwise", () => {
    const chip = subscriptionPeriodChip(t, {
      status: "active",
      period_ends_at: "2026-10-08T19:29:15.000Z",
      uses_balance: true,
    });
    expect(chip?.label).toContain("usage:subscriptionRenews");
  });

  it("returns null without a period end", () => {
    expect(subscriptionPeriodChip(t, { status: "inactive", uses_balance: false })).toBeNull();
  });
});
