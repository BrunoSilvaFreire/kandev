"use client";

import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@kandev/ui/table";
import { Badge } from "@kandev/ui/badge";
import { IconArrowDown, IconArrowUp } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { formatDollars } from "@/lib/utils";
import { formatNumber } from "@/lib/usage/format";
import { parseStrictRfc3339Timestamp } from "@/lib/utils/strict-timestamp";
import type {
  UsageBreakdownQuery,
  UsageBreakdownRow,
  UsageBreakdownSort,
} from "@/lib/types/provider-usage";
import TaskLink from "@/components/routing/task-link";
import { sharePct } from "./usage-breakdown-model";

export function rowLabel(row: UsageBreakdownRow, groupBy: UsageBreakdownQuery["groupBy"]): string {
  if (row.label) return row.label;
  if (groupBy === "session") return row.session_name || row.key;
  return row.key;
}

function formatWhen(value: string): string | null {
  const parsed = parseStrictRfc3339Timestamp(value);
  if (parsed === null) return null;
  return new Date(Number(parsed / BigInt(1_000_000))).toLocaleString();
}

function ariaSortValue(active: boolean, order: string): "ascending" | "descending" | "none" {
  if (!active) return "none";
  return order === "asc" ? "ascending" : "descending";
}

function SortableHead({
  column,
  label,
  query,
  onSort,
  align,
}: {
  column: UsageBreakdownSort;
  label: string;
  query: UsageBreakdownQuery;
  onSort: (column: UsageBreakdownSort) => void;
  align?: "right";
}) {
  const active = (query.sort ?? "tokens") === column;
  const order = query.order ?? "desc";
  const ariaSort = ariaSortValue(active, order);
  return (
    <TableHead aria-sort={ariaSort} className={align === "right" ? "text-right" : undefined}>
      <button
        type="button"
        onClick={() => onSort(column)}
        className="inline-flex cursor-pointer items-center gap-1"
      >
        {label}
        {active &&
          (order === "asc" ? (
            <IconArrowUp className="h-3 w-3" />
          ) : (
            <IconArrowDown className="h-3 w-3" />
          ))}
      </button>
    </TableHead>
  );
}

function RowLabelCell({
  row,
  groupBy,
  onDrill,
}: {
  row: UsageBreakdownRow;
  groupBy: UsageBreakdownQuery["groupBy"];
  onDrill: () => void;
}) {
  const { t } = useTranslation();
  const label = rowLabel(row, groupBy);
  if (groupBy === "session" && row.task_id) {
    return (
      <TableCell className="max-w-[14rem]">
        <TaskLink
          taskId={row.task_id}
          sessionId={row.key === row.task_id ? undefined : row.key}
          className="block truncate font-medium hover:underline"
          onClick={(event) => event.stopPropagation()}
        >
          {label}
        </TaskLink>
        <span className="text-xs text-muted-foreground">{t("usage:breakdownOpenTask")}</span>
      </TableCell>
    );
  }
  return (
    <TableCell className="max-w-[14rem]">
      <button
        type="button"
        onClick={onDrill}
        className="cursor-pointer truncate text-left font-medium"
      >
        {label}
      </button>
    </TableCell>
  );
}

function TokenShare({ value, total }: { value: number; total: number }) {
  const pct = sharePct(value, total);
  return (
    <div className="flex items-center gap-2">
      <div className="h-1.5 w-16 overflow-hidden rounded-full bg-muted">
        <div className="h-full rounded-full bg-blue-500" style={{ width: `${pct}%` }} />
      </div>
      <span className="tabular-nums">{pct.toFixed(0)}%</span>
    </div>
  );
}

function BreakdownRow({
  row,
  groupBy,
  totalTokens,
  onDrill,
}: {
  row: UsageBreakdownRow;
  groupBy: UsageBreakdownQuery["groupBy"];
  totalTokens: number;
  onDrill: (row: UsageBreakdownRow) => void;
}) {
  const { t } = useTranslation();
  return (
    <TableRow
      role="button"
      tabIndex={0}
      data-testid="usage-breakdown-row"
      onClick={() => onDrill(row)}
      onKeyDown={(event) => {
        if (event.key === "Enter") onDrill(row);
      }}
      className="cursor-pointer"
    >
      <RowLabelCell row={row} groupBy={groupBy} onDrill={() => onDrill(row)} />
      {groupBy === "session" && (
        <TableCell>
          <div className="flex flex-wrap gap-1">
            {row.provider && <Badge variant="outline">{row.provider}</Badge>}
            {row.model && <Badge variant="secondary">{row.model}</Badge>}
            {row.models > 1 && (
              <Badge variant="secondary">
                {t("usage:breakdownMoreModels", { count: row.models - 1 })}
              </Badge>
            )}
          </div>
        </TableCell>
      )}
      <TableCell className="tabular-nums">
        <div className="flex items-center gap-2">
          <span>{formatNumber(row.tokens_total)}</span>
          <TokenShare value={row.tokens_total} total={totalTokens} />
        </div>
      </TableCell>
      <TableCell className="text-right tabular-nums">{formatNumber(row.tokens_in)}</TableCell>
      <TableCell className="text-right tabular-nums">{formatNumber(row.tokens_out)}</TableCell>
      <TableCell className="text-right tabular-nums">
        {formatNumber(row.tokens_cached_read)}
      </TableCell>
      <TableCell className="text-right tabular-nums">{formatDollars(row.cost_subcents)}</TableCell>
      <TableCell className="text-right tabular-nums">{row.events}</TableCell>
      <TableCell className="text-right text-xs text-muted-foreground">
        {formatWhen(row.last_at) ?? row.last_at}
      </TableCell>
    </TableRow>
  );
}

/** Desktop Breakdown table with sortable columns and row drill-down. */
export function UsageBreakdownTable({
  rows,
  query,
  totalTokens,
  onSort,
  onDrill,
}: {
  rows: UsageBreakdownRow[];
  query: UsageBreakdownQuery;
  totalTokens: number;
  onSort: (column: UsageBreakdownSort) => void;
  onDrill: (row: UsageBreakdownRow) => void;
}) {
  const { t } = useTranslation();
  const groupBy = query.groupBy ?? "session";
  return (
    <div className="overflow-x-auto rounded-md border" data-testid="usage-breakdown-table">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t("usage:breakdownColumnName")}</TableHead>
            {groupBy === "session" && <TableHead>{t("usage:breakdownColumnProvider")}</TableHead>}
            <SortableHead
              column="tokens"
              label={t("usage:breakdownColumnTokens")}
              query={query}
              onSort={onSort}
            />
            <TableHead className="text-right">{t("usage:breakdownColumnTokensIn")}</TableHead>
            <TableHead className="text-right">{t("usage:breakdownColumnTokensOut")}</TableHead>
            <TableHead className="text-right">{t("usage:breakdownColumnCached")}</TableHead>
            <SortableHead
              column="cost"
              label={t("usage:breakdownColumnCost")}
              query={query}
              onSort={onSort}
              align="right"
            />
            <SortableHead
              column="events"
              label={t("usage:breakdownColumnEvents")}
              query={query}
              onSort={onSort}
              align="right"
            />
            <SortableHead
              column="last_at"
              label={t("usage:breakdownColumnLast")}
              query={query}
              onSort={onSort}
              align="right"
            />
          </TableRow>
        </TableHeader>
        <TableBody>
          {rows.map((row) => (
            <BreakdownRow
              key={row.key}
              row={row}
              groupBy={groupBy}
              totalTokens={totalTokens}
              onDrill={onDrill}
            />
          ))}
        </TableBody>
      </Table>
    </div>
  );
}
