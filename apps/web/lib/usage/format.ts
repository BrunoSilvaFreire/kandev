/** Compact integer formatting for token counts (1.2K, 3.4M). */
export function formatNumber(num: number): string {
  if (num >= 1_000_000) {
    return `${(num / 1_000_000).toFixed(1)}M`;
  }
  if (num >= 1_000) {
    return `${(num / 1_000).toFixed(1)}K`;
  }
  return num.toLocaleString();
}

/** Cache hit ratio as a whole percentage, or null when it is not reported. */
export function formatHitRatio(ratio: number | null): string | null {
  return ratio === null ? null : `${Math.round(ratio * 100)}%`;
}

/** i18n key for a cache status label, shared by the badge and the inspector. */
export function cacheStatusLabelKey(status: "warm" | "likely_expired" | "unknown"): string {
  switch (status) {
    case "warm":
      return "task:usageInspectorStatusWarm";
    case "likely_expired":
      return "task:usageInspectorStatusLikelyExpired";
    default:
      return "task:usageInspectorStatusUnknown";
  }
}
