import type { TFunction } from "i18next";
import { formatDollars } from "@/lib/utils";
import { cacheStatusLabelKey, formatHitRatio, formatNumber } from "@/lib/usage/format";
import { usageRowStats } from "@/lib/usage/breakdown";
import { usageFlags } from "@/lib/usage/efficiency";
import { parseStrictRfc3339Timestamp } from "@/lib/utils/strict-timestamp";
import type { UsageDisplayRow } from "./usage-panel-rows";

export type UsageMetric = { label: string; value: string; note?: string };

/** Cost string with a `~` prefix when any contributing event was estimated. */
export function costText(row: UsageDisplayRow): string {
  return `${row.totals.estimated_event_count > 0 ? "~" : ""}${formatDollars(row.totals.cost_subcents)}`;
}

function incompleteNote(row: UsageDisplayRow, t: TFunction): string | null {
  return row.totals.output_tokens_complete ? null : t("task:usageInspectorIncomplete");
}

/** Estimated/unpriced/incomplete note for a row, or null when nothing to flag. */
export function noteText(row: UsageDisplayRow, t: TFunction): string | null {
  const parts = [
    row.totals.estimated_event_count > 0 ? t("task:usageInspectorEstimated") : null,
    row.totals.unpriced_event_count > 0
      ? t("task:usageInspectorUnpriced", { count: row.totals.unpriced_event_count })
      : null,
    incompleteNote(row, t),
  ].filter((value): value is string => value !== null);
  return parts.length > 0 ? parts.join(", ") : null;
}

/** The Part B per-row warnings (low hit ratio, high cost, expired cache). */
export function rowFlagsText(row: UsageDisplayRow, t: TFunction): string | null {
  const flags = usageFlags({ totals: row.totals, cacheStatus: row.cacheStatus ?? "unknown" });
  const parts = [
    flags.lowHitRatio ? t("task:usageInspectorFlagLowHitRatio") : null,
    flags.highCost ? t("task:usageInspectorFlagHighCost") : null,
    flags.cacheLikelyExpired ? t("task:usageInspectorFlagCacheExpired") : null,
  ].filter((value): value is string => value !== null);
  return parts.length > 0 ? parts.join(" · ") : null;
}

function formatEventTime(value: string | null, fallback: string): string {
  if (!value) return fallback;
  const parsed = parseStrictRfc3339Timestamp(value);
  if (parsed === null) return fallback;
  return new Date(Number(parsed / BigInt(1_000_000))).toLocaleString();
}

/**
 * The metric fields shared by the table and the stacked phone cards, so
 * desktop and mobile always show the same set. Session rows add the estimated
 * cache status, expiry and live last-prompt tokens.
 */
export function metricsFor(row: UsageDisplayRow, t: TFunction): UsageMetric[] {
  const stats = usageRowStats(row.totals);
  const notAvailable = t("task:usagePanelNotAvailable");
  const metrics: UsageMetric[] = [
    { label: t("task:usagePanelPrompts"), value: String(row.totals.event_count) },
    { label: t("task:usageInspectorInput"), value: formatNumber(row.totals.tokens_in) },
    { label: t("task:usageInspectorCachedRead"), value: formatNumber(row.totals.tokens_cached_read) },
    { label: t("task:usageInspectorCachedWrite"), value: formatNumber(row.totals.tokens_cached_write) },
    { label: t("task:usageInspectorOutput"), value: formatNumber(row.totals.tokens_out) },
    { label: t("task:usageInspectorThought"), value: formatNumber(row.totals.tokens_thought) },
    { label: t("task:usageInspectorTotal"), value: formatNumber(row.totals.tokens_total) },
    {
      label: t("task:usageInspectorHitRatio"),
      value: formatHitRatio(stats.hitRatio) ?? t("task:usageInspectorHitRatioNotReported"),
    },
    { label: t("task:usageInspectorCostTitle"), value: costText(row) },
    {
      label: t("task:usagePanelCostPerPrompt"),
      value: stats.costPerPrompt === null ? notAvailable : formatDollars(stats.costPerPrompt),
    },
    {
      label: t("task:usagePanelAvgInput"),
      value:
        stats.avgInputPerPrompt === null ? notAvailable : formatNumber(Math.round(stats.avgInputPerPrompt)),
    },
    {
      label: t("task:usagePanelAvgOutput"),
      value:
        stats.avgOutputPerPrompt === null
          ? notAvailable
          : formatNumber(Math.round(stats.avgOutputPerPrompt)),
    },
    { label: t("task:usagePanelFirstEvent"), value: formatEventTime(row.totals.first_event_at, notAvailable) },
    { label: t("task:usagePanelLastEvent"), value: formatEventTime(row.totals.last_event_at, notAvailable) },
  ];
  if (row.isSession) {
    metrics.push(
      {
        label: t("task:usageInspectorCacheTitle"),
        value: row.cacheStatus ? t(cacheStatusLabelKey(row.cacheStatus)) : notAvailable,
        note: t("task:usageInspectorEstimateNote"),
      },
      {
        label: t("task:usageInspectorExpiresAt"),
        value: row.expiresAt === null ? notAvailable : new Date(row.expiresAt).toLocaleString(),
      },
      {
        label: t("task:usageInspectorLastPrompt"),
        value: row.lastPrompt ? formatNumber(row.lastPrompt.totalTokens) : notAvailable,
      },
    );
  }
  return metrics;
}
