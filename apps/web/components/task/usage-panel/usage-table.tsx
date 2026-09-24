"use client";

import { useTranslation } from "react-i18next";
import { metricsFor, noteText, rowFlagsText } from "./usage-panel-format";
import type { UsageDisplayRow } from "./usage-panel-rows";

const NUMERIC_CELL = "px-2 py-1 text-right tabular-nums";
const HEADER_CELL = "px-2 py-1 text-left font-medium";

function RowName({ row }: { row: UsageDisplayRow }) {
  const { t } = useTranslation();
  const deletedSession = row.isSession && row.sessionId === null;
  const flags = rowFlagsText(row, t);
  const note = noteText(row, t);
  return (
    <td className="min-w-40 px-2 py-1">
      <div className="truncate text-foreground">
        {deletedSession ? t("task:usagePanelDeletedSession") : row.primary}
      </div>
      {row.secondary ? (
        <div className="truncate text-[10px] text-muted-foreground">{row.secondary}</div>
      ) : null}
      {flags ? (
        <div className="text-[9px] text-amber-500" data-testid="usage-panel-row-flags">
          {flags}
        </div>
      ) : null}
      {note ? (
        <div className="text-[9px] text-muted-foreground/80" data-testid="usage-panel-row-note">
          {note}
        </div>
      ) : null}
    </td>
  );
}

/** Detail table for a viewport at or above the mobile boundary. */
export function UsageTable({ rows }: { rows: UsageDisplayRow[] }) {
  const { t } = useTranslation();
  // Every row in one view carries the same metric set, so the first row's
  // labels describe the whole table.
  const headerMetrics = rows.length > 0 ? metricsFor(rows[0], t) : [];
  return (
    <div className="overflow-x-auto" data-testid="usage-panel-table">
      <table className="w-full border-collapse text-xs">
        <thead>
          <tr className="border-b border-border text-[10px] uppercase text-muted-foreground">
            <th className={HEADER_CELL}>{t("task:usagePanelName")}</th>
            {headerMetrics.map((metric) => (
              <th key={metric.label} className={HEADER_CELL}>
                {metric.label}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => (
            <tr key={row.key} data-testid="usage-panel-row" className="border-b border-border/60">
              <RowName row={row} />
              {metricsFor(row, t).map((metric) => (
                <td key={metric.label} className={NUMERIC_CELL}>
                  {metric.value}
                  {metric.note ? (
                    <div className="text-[9px] text-muted-foreground/80">{metric.note}</div>
                  ) : null}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
