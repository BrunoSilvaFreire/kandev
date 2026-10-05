"use client";

import { useTranslation } from "react-i18next";
import type { ProviderUsageAccount } from "@/lib/types/provider-usage";
import { UsageInfoChip } from "./usage-info-chip";
import {
  formatBalance,
  subscriptionPeriodChip,
  subscriptionStatusKey,
  subscriptionStatusTone,
} from "./usage-facts-format";

/**
 * The account's informational facts: the provider-reported subscription state
 * and any prepaid balances. These never feed the utilization windows.
 */
export function UsageAccountFacts({ account }: { account: ProviderUsageAccount }) {
  const { t } = useTranslation();
  const subscription = account.subscription;
  const balances = account.balances ?? [];
  if (!subscription && balances.length === 0) return null;
  const statusLabel = subscription ? t(subscriptionStatusKey(subscription.status)) : "";
  const period = subscription ? subscriptionPeriodChip(t, subscription) : null;
  return (
    <div className="space-y-2" data-testid="usage-account-facts">
      {subscription && (
        <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
          <UsageInfoChip
            label={statusLabel}
            tooltip={statusLabel}
            tone={subscriptionStatusTone(subscription.status)}
            testId="usage-subscription-chip"
          />
          {period && (
            <UsageInfoChip
              label={period.label}
              tooltip={period.tooltip}
              testId="usage-subscription-period-chip"
            />
          )}
          {subscription.uses_balance && (
            <UsageInfoChip
              label={t("usage:subscriptionUsesBalance")}
              tooltip={t("usage:subscriptionUsesBalanceTooltip")}
              testId="usage-subscription-balance-chip"
            />
          )}
        </div>
      )}
      {balances.map((balance) => (
        <div
          key={balance.label}
          className="flex items-center justify-between text-xs"
          data-testid={`usage-balance-${balance.label}`}
        >
          <span className="text-muted-foreground">
            {t("usage:balanceLine", { label: balance.label.toUpperCase() })}
          </span>
          <span className="font-medium">{formatBalance(t, balance)}</span>
        </div>
      ))}
    </div>
  );
}
