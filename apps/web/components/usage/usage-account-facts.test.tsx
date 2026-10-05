import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { ProviderUsageAccount } from "@/lib/types/provider-usage";
import { UsageAccountFacts } from "./usage-account-facts";

afterEach(cleanup);

function renderFacts(account: ProviderUsageAccount) {
  return render(
    <TooltipProvider>
      <UsageAccountFacts account={account} />
    </TooltipProvider>,
  );
}

const base: ProviderUsageAccount = {
  account_key: "junie-account",
  status: "healthy",
  profiles: [],
  windows: [],
};

describe("UsageAccountFacts", () => {
  it("renders the subscription status and period chips", () => {
    renderFacts({
      ...base,
      subscription: {
        status: "canceling",
        period_ends_at: "2026-10-08T19:29:15.000Z",
        uses_balance: true,
      },
    });
    expect(screen.getByTestId("usage-subscription-chip")).toBeTruthy();
    expect(screen.getByTestId("usage-subscription-period-chip")).toBeTruthy();
    expect(screen.getByTestId("usage-subscription-balance-chip")).toBeTruthy();
  });

  it("renders one row per balance", () => {
    renderFacts({
      ...base,
      balances: [
        { label: "aip", amount: 993478.33, unit: "CREDITS" },
        { label: "prepaid", amount: 6.1, unit: "USD" },
      ],
    });
    expect(screen.getByTestId("usage-balance-aip")).toBeTruthy();
    expect(screen.getByTestId("usage-balance-prepaid")).toBeTruthy();
  });

  it("renders nothing without facts", () => {
    const { container } = renderFacts(base);
    expect(container.querySelector("[data-testid='usage-account-facts']")).toBeNull();
  });
});
