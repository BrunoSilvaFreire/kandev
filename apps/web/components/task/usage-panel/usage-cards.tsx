"use client";

import { useTranslation } from "react-i18next";
import { metricsFor, noteText, rowFlagsText } from "./usage-panel-format";
import type { UsageDisplayRow } from "./usage-panel-rows";

/** Stacked cards for a viewport below the mobile boundary (no wide table). */
export function UsageCards({ rows }: { rows: UsageDisplayRow[] }) {
  const { t } = useTranslation();
  return (
    <div className="space-y-2" data-testid="usage-panel-cards">
      {rows.map((row) => {
        const note = noteText(row, t);
        const flags = rowFlagsText(row, t);
        const deletedSession = row.isSession && row.sessionId === null;
        return (
          <div
            key={row.key}
            data-testid="usage-panel-card"
            className="rounded border border-border p-3"
          >
            <div className="min-w-0">
              <div className="truncate text-sm font-medium text-foreground">
                {deletedSession ? t("task:usagePanelDeletedSession") : row.primary}
              </div>
              {row.secondary ? (
                <div className="truncate text-[10px] text-muted-foreground">{row.secondary}</div>
              ) : null}
              {flags ? (
                <div className="text-[10px] text-amber-500" data-testid="usage-panel-card-flags">
                  {flags}
                </div>
              ) : null}
            </div>
            <dl className="mt-2 grid grid-cols-2 gap-x-4 gap-y-1">
              {metricsFor(row, t).map((metric) => (
                <div
                  key={metric.label}
                  className="flex min-w-0 items-baseline justify-between gap-2"
                >
                  <dt className="truncate text-[10px] text-muted-foreground">{metric.label}</dt>
                  <dd className="shrink-0 text-right tabular-nums text-xs text-foreground">
                    {metric.value}
                    {metric.note ? (
                      <div className="text-[9px] text-muted-foreground/80">{metric.note}</div>
                    ) : null}
                  </dd>
                </div>
              ))}
            </dl>
            {note ? <div className="mt-1 text-[10px] text-muted-foreground">{note}</div> : null}
          </div>
        );
      })}
    </div>
  );
}
