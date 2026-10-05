import type { TFunction } from "i18next";
import type {
  ProviderUsageConfidence,
  ProviderUsageEstimate,
  ProviderUsageHistoryPoint,
  ProviderUsageQuotaUnavailableReason,
  ProviderUsageTrend,
} from "@/lib/types/provider-usage";
import { parseStrictRfc3339Timestamp } from "@/lib/utils/strict-timestamp";

/** Which provenance badge a window carries. */
export type UsageSourceBadge = "reported" | "estimated";

/** Chip tone, reusing the status-chip palette. */
export type UsageChipTone = "neutral" | "healthy" | "nearing" | "exhausted";

/** A chip with an explanatory tooltip. */
export type UsageChipData = { label: string; tooltip: string };

/** A keyed estimate chip carrying its tone. */
export type UsageInfoChipData = UsageChipData & { key: string; tone: UsageChipTone };

/** A measured window is provider-reported; an estimate series is derived. */
export function windowSourceBadge(source: "measured" | "estimated"): UsageSourceBadge {
  return source === "estimated" ? "estimated" : "reported";
}

/** i18n key for a closed-set unavailable reason. */
export function quotaUnavailableReasonKey(reason: ProviderUsageQuotaUnavailableReason): string {
  if (reason === "api_key_billing") return "usage:quotaReasonApiKeyBilling";
  if (reason === "credentials_missing") return "usage:quotaReasonCredentialsMissing";
  if (reason === "credentials_expired") return "usage:quotaReasonCredentialsExpired";
  if (reason === "profile_missing") return "usage:quotaReasonProfileMissing";
  return "usage:quotaReasonNoProviderEndpoint";
}

/** Newest observation in a window series, or null when it has none. */
export function lastFetchedAt(history: ProviderUsageHistoryPoint[]): string | null {
  let latest: bigint | null = null;
  for (const point of history) {
    const parsed = parseStrictRfc3339Timestamp(point.at);
    if (parsed !== null && (latest === null || parsed > latest)) latest = parsed;
  }
  return latest === null ? null : new Date(Number(latest / BigInt(1_000_000))).toLocaleString();
}

function trendLabelKey(trend: ProviderUsageTrend): string {
  if (trend === "rising") return "usage:trendRising";
  if (trend === "falling") return "usage:trendFalling";
  return "usage:trendFlat";
}

function trendTooltipKey(trend: ProviderUsageTrend): string {
  if (trend === "rising") return "usage:trendRisingTooltip";
  if (trend === "falling") return "usage:trendFallingTooltip";
  return "usage:trendFlatTooltip";
}

function confidenceKey(confidence: ProviderUsageConfidence): string {
  if (confidence === "medium") return "usage:confidenceMedium";
  if (confidence === "low") return "usage:confidenceLow";
  return "usage:confidenceNone";
}

function trendTone(trend: ProviderUsageTrend): UsageChipTone {
  if (trend === "rising") return "nearing";
  if (trend === "falling") return "healthy";
  return "neutral";
}

/** Bar color for a utilization percentage. */
export function utilizationBarColor(pct: number): string {
  if (pct >= 90) return "bg-red-500";
  if (pct >= 80) return "bg-amber-500";
  return "bg-blue-500";
}

/** Text color for a utilization percentage. */
export function utilizationTextColor(pct: number): string {
  if (pct >= 90) return "text-red-600 dark:text-red-400";
  if (pct >= 80) return "text-amber-600 dark:text-amber-400";
  return "text-muted-foreground";
}

/** Clamp a percentage to the 0-100 bar range. */
export function clampPct(pct: number): number {
  return Math.min(100, Math.max(0, pct));
}

/** Render a local reset countdown. */
export function formatResetCountdown(t: TFunction, resetAt?: string): string {
  if (!resetAt) return t("usage:noReset");
  const diff = new Date(resetAt).getTime() - Date.now();
  if (diff <= 0) return t("usage:resettingSoon");
  const hours = Math.floor(diff / 3_600_000);
  const minutes = Math.floor((diff % 3_600_000) / 60_000);
  if (hours > 0) return t("usage:resetsInHoursMinutes", { hours, minutes });
  return t("usage:resetsInMinutes", { minutes });
}

/** The reset chip: countdown label plus the exact local reset time. */
export function resetChip(t: TFunction, resetAt?: string): UsageChipData {
  return {
    label: formatResetCountdown(t, resetAt),
    tooltip: resetAt
      ? t("usage:resetTooltip", { at: new Date(resetAt).toLocaleString() })
      : t("usage:resetTooltipNone"),
  };
}

/**
 * The last-fetched chip, or null when the series has no parseable point. Its
 * tooltip carries the absolute time and how the window was sourced.
 */
export function fetchedChip(
  t: TFunction,
  history: ProviderUsageHistoryPoint[],
  source: "measured" | "estimated",
): UsageChipData | null {
  const at = lastFetchedAt(history);
  if (!at) return null;
  const provenance =
    windowSourceBadge(source) === "estimated"
      ? t("usage:provenanceEstimated")
      : t("usage:provenanceReported");
  return {
    label: t("usage:lastFetchedAt", { at }),
    tooltip: t("usage:fetchedTooltip", { at, provenance }),
  };
}

/**
 * Build the estimate chips. Always labelled estimated; a single "not enough
 * data" chip replaces the estimate when the confidence is none.
 */
export function estimateChips(t: TFunction, estimate?: ProviderUsageEstimate): UsageInfoChipData[] {
  if (!estimate || estimate.confidence === "none") {
    return [
      {
        key: "notEnough",
        label: t("usage:estimateNotEnough"),
        tooltip: t("usage:estimateNotEnoughTooltip"),
        tone: "neutral",
      },
    ];
  }
  const rate = estimate.burn_rate_pct_per_hour.toFixed(1);
  const chips: UsageInfoChipData[] = [
    {
      key: "burn",
      label: t("usage:estimateBurnRate", { rate }),
      tooltip: t("usage:estimateBurnRateTooltip", {
        rate,
        confidence: t(confidenceKey(estimate.confidence)),
      }),
      tone: "nearing",
    },
    {
      key: "trend",
      label: t(trendLabelKey(estimate.trend)),
      tooltip: t(trendTooltipKey(estimate.trend)),
      tone: trendTone(estimate.trend),
    },
  ];
  chips.push(exhaustionChip(t, estimate));
  return chips;
}

function exhaustionChip(t: TFunction, estimate: ProviderUsageEstimate): UsageInfoChipData {
  const resetAt = estimate.reset_at
    ? new Date(estimate.reset_at).toLocaleString()
    : t("usage:noReset");
  if (estimate.exhausts_before_reset && estimate.projected_exhaustion_at) {
    const at = new Date(estimate.projected_exhaustion_at).toLocaleString();
    return {
      key: "exhaustion",
      label: t("usage:estimateExhaustsAt", { at }),
      tooltip: t("usage:estimateExhaustsTooltip", { at, resetAt }),
      tone: "exhausted",
    };
  }
  return {
    key: "resets",
    label: t("usage:estimateResetsFirst"),
    tooltip: t("usage:estimateResetsFirstTooltip", { resetAt }),
    tone: "healthy",
  };
}
