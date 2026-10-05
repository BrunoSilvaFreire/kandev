"use client";

import { Card } from "@kandev/ui/card";
import { Badge } from "@kandev/ui/badge";
import { useTranslation } from "react-i18next";
import { formatDollars } from "@/lib/utils";
import { formatNumber } from "@/lib/usage/format";
import { parseStrictRfc3339Timestamp } from "@/lib/utils/strict-timestamp";
import type { UsageBreakdownQuery, UsageBreakdownRow } from "@/lib/types/provider-usage";
import TaskLink from "@/components/routing/task-link";
import { rowLabel } from "./usage-breakdown-table";
import { sharePct } from "./usage-breakdown-model";

function formatWhen(value: string): string {
  const parsed = parseStrictRfc3339Timestamp(value);
  if (parsed === null) return value;
  return new Date(Number(parsed / BigInt(1_000_000))).toLocaleDateString();
}

/** Phone composition: one stacked card per breakdown row. */
export function UsageBreakdownCards({
  rows,
  query,
  totalTokens,
  onDrill,
}: {
  rows: UsageBreakdownRow[];
  query: UsageBreakdownQuery;
  totalTokens: number;
  onDrill: (row: UsageBreakdownRow) => void;
}) {
  const { t } = useTranslation();
  const groupBy = query.groupBy ?? "session";
  return (
    <div className="space-y-2" data-testid="usage-breakdown-cards">
      {rows.map((row) => {
        const label = rowLabel(row, groupBy);
        return (
          <Card key={row.key} className="space-y-2 p-3" data-testid="usage-breakdown-card">
            <div className="flex items-start justify-between gap-2">
              {groupBy === "session" && row.task_id ? (
                <TaskLink
                  taskId={row.task_id}
                  sessionId={row.key === row.task_id ? undefined : row.key}
                  className="min-w-0 flex-1 truncate font-medium hover:underline"
                >
                  {label}
                </TaskLink>
              ) : (
                <button
                  type="button"
                  onClick={() => onDrill(row)}
                  className="min-w-0 flex-1 cursor-pointer truncate text-left font-medium"
                >
                  {label}
                </button>
              )}
              <button
                type="button"
                onClick={() => onDrill(row)}
                aria-label={t("usage:breakdownDrillDown", { name: label })}
                className="shrink-0 cursor-pointer text-xs text-muted-foreground"
              >
                {t("usage:breakdownDrillDownShort")}
              </button>
            </div>
            <div className="flex flex-wrap gap-1">
              {row.provider && <Badge variant="outline">{row.provider}</Badge>}
              {row.model && <Badge variant="secondary">{row.model}</Badge>}
            </div>
            <div className="flex items-center gap-2 text-sm">
              <span className="tabular-nums">{formatNumber(row.tokens_total)}</span>
              <span className="text-xs text-muted-foreground">
                {t("usage:breakdownShareOfTotal", {
                  pct: sharePct(row.tokens_total, totalTokens).toFixed(0),
                })}
              </span>
            </div>
            <div className="grid grid-cols-3 gap-1 text-xs text-muted-foreground">
              <span>
                {t("usage:breakdownCardCost", { value: formatDollars(row.cost_subcents) })}
              </span>
              <span>{t("usage:breakdownCardEvents", { count: row.events })}</span>
              <span>{t("usage:breakdownCardLast", { at: formatWhen(row.last_at) })}</span>
            </div>
          </Card>
        );
      })}
    </div>
  );
}
