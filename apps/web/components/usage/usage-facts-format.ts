import type { TFunction } from "i18next";
import type {
  ProviderUsageBalance,
  ProviderUsageSubscription,
  ProviderUsageSubscriptionStatus,
} from "@/lib/types/provider-usage";
import type { UsageChipData, UsageChipTone } from "./usage-format";

/** i18n key for a closed-set subscription status. */
export function subscriptionStatusKey(status: ProviderUsageSubscriptionStatus): string {
  if (status === "active") return "usage:subscriptionActive";
  if (status === "canceling") return "usage:subscriptionCanceling";
  if (status === "renewal_pending") return "usage:subscriptionRenewalPending";
  return "usage:subscriptionInactive";
}

/** Chip tone for a subscription status. */
export function subscriptionStatusTone(status: ProviderUsageSubscriptionStatus): UsageChipTone {
  if (status === "active") return "healthy";
  if (status === "canceling" || status === "renewal_pending") return "nearing";
  return "neutral";
}

/**
 * The period chip: "ends" for a canceling subscription, "renews" otherwise. A
 * subscription without a period end renders no chip.
 */
export function subscriptionPeriodChip(
  t: TFunction,
  subscription: ProviderUsageSubscription,
): UsageChipData | null {
  if (!subscription.period_ends_at) return null;
  const date = new Date(subscription.period_ends_at).toLocaleDateString();
  const label =
    subscription.status === "canceling"
      ? t("usage:subscriptionEnds", { date })
      : t("usage:subscriptionRenews", { date });
  return { label, tooltip: label };
}

/** Render a prepaid balance: currency for USD, grouped credits, else raw unit. */
export function formatBalance(t: TFunction, balance: ProviderUsageBalance): string {
  if (balance.unit === "USD") {
    return new Intl.NumberFormat(undefined, { style: "currency", currency: "USD" }).format(
      balance.amount,
    );
  }
  const grouped = new Intl.NumberFormat(undefined, { maximumFractionDigits: 2 }).format(
    balance.amount,
  );
  if (balance.unit === "CREDITS") {
    return t("usage:balanceUnitCredits", { amount: grouped });
  }
  return `${grouped} ${balance.unit}`;
}
