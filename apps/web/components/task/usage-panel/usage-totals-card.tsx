"use client";

import { useTranslation } from "react-i18next";
import type { UsageTotals } from "@/lib/api/domains/usage-api";
import { hitRatio, usageFlags } from "@/lib/usage/efficiency";
import { formatHitRatio, formatNumber } from "@/lib/usage/format";
import { formatDollars } from "@/lib/utils";

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex min-w-0 flex-col">
      <span className="truncate text-[10px] uppercase text-muted-foreground">{label}</span>
      <span className="tabular-nums text-sm font-medium text-foreground">{value}</span>
    </div>
  );
}

/** Header card with the task-wide totals and any raised flag. */
export function UsageTotalsCard({ totals }: { totals: UsageTotals | null }) {
  const { t } = useTranslation();
  if (!totals) return null;
  if (totals.event_count === 0) {
    return (
      <div
        data-testid="usage-panel-totals"
        className="rounded border border-border bg-muted/30 px-3 py-2 text-xs text-muted-foreground"
      >
        {t("task:usagePanelNoUsage")}
      </div>
    );
  }

  const flags = usageFlags({ totals, cacheStatus: "unknown" });
  const raised = [
    flags.lowHitRatio ? t("task:usageInspectorFlagLowHitRatio") : null,
    flags.highCost ? t("task:usageInspectorFlagHighCost") : null,
  ].filter((value): value is string => value !== null);

  return (
    <div data-testid="usage-panel-totals" className="rounded border border-border bg-muted/30 px-3 py-2">
      <div className="mb-2 text-[10px] font-medium uppercase text-muted-foreground">
        {t("task:usagePanelTaskTotal")}
      </div>
      <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
        <Metric label={t("task:usagePanelPrompts")} value={String(totals.event_count)} />
        <Metric label={t("task:usageInspectorTotal")} value={formatNumber(totals.tokens_total)} />
        <Metric
          label={t("task:usageInspectorHitRatio")}
          value={formatHitRatio(hitRatio(totals)) ?? t("task:usageInspectorHitRatioNotReported")}
        />
        <Metric
          label={t("task:usageInspectorCostTitle")}
          value={`${totals.estimated_event_count > 0 ? "~" : ""}${formatDollars(totals.cost_subcents)}`}
        />
      </div>
      {raised.length > 0 ? (
        <div className="mt-2 text-[11px] text-amber-500" data-testid="usage-panel-flags">
          {raised.join(" · ")}
        </div>
      ) : null}
    </div>
  );
}
