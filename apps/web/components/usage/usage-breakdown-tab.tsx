"use client";

import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useUsageBreakdown } from "@/hooks/domains/usage/use-usage-breakdown";
import { formatDollars } from "@/lib/utils";
import { formatNumber } from "@/lib/usage/format";
import type {
  ProviderUsageRange,
  UsageBreakdownQuery,
  UsageBreakdownResponse,
  UsageBreakdownRow,
  UsageBreakdownSort,
} from "@/lib/types/provider-usage";
import { UsageBreakdownCards } from "./usage-breakdown-cards";
import { UsageBreakdownFilters } from "./usage-breakdown-filters";
import { UsageBreakdownTable } from "./usage-breakdown-table";
import { UsageBreakdownToolbar } from "./usage-breakdown-toolbar";
import { drillDown } from "./usage-breakdown-model";

const DEFAULT_LIMIT = 50;

/** Sort header click: toggle direction on the active column, else sort desc. */
function nextSort(
  query: UsageBreakdownQuery,
  column: UsageBreakdownSort,
): Partial<UsageBreakdownQuery> {
  if ((query.sort ?? "tokens") === column) {
    return { order: (query.order ?? "desc") === "desc" ? "asc" : "desc" };
  }
  return { sort: column, order: "desc" };
}

function SummaryChips({ data }: { data: UsageBreakdownResponse }) {
  const { t } = useTranslation();
  return (
    <div
      className="flex flex-wrap items-center gap-2 text-xs"
      data-testid="usage-breakdown-summary"
    >
      <Badge variant="secondary">
        {t("usage:breakdownTotalTokens", { value: formatNumber(data.total_tokens) })}
      </Badge>
      <Badge variant="secondary">
        {t("usage:breakdownTotalCost", { value: formatDollars(data.total_cost_subcents) })}
      </Badge>
      <Badge variant="secondary">
        {t("usage:breakdownTotalEvents", { count: data.total_events })}
      </Badge>
      <Badge variant="outline">{t("usage:breakdownTotalRows", { count: data.total_rows })}</Badge>
    </div>
  );
}

function BreakdownResults({
  data,
  loading,
  error,
  isMobile,
  query,
  onSort,
  onDrill,
}: {
  data: UsageBreakdownResponse | null;
  loading: boolean;
  error: boolean;
  isMobile: boolean;
  query: UsageBreakdownQuery;
  onSort: (column: UsageBreakdownSort) => void;
  onDrill: (row: UsageBreakdownRow) => void;
}) {
  const { t } = useTranslation();
  const rows = data?.rows ?? [];
  if (loading && rows.length === 0) {
    return (
      <p role="status" className="text-sm text-muted-foreground">
        {t("usage:breakdownLoading")}
      </p>
    );
  }
  if (error) {
    return <p className="text-sm text-muted-foreground">{t("usage:breakdownError")}</p>;
  }
  if (rows.length === 0) {
    return <p className="text-sm text-muted-foreground">{t("usage:breakdownEmpty")}</p>;
  }
  const totalTokens = data?.total_tokens ?? 0;
  if (isMobile) {
    return (
      <UsageBreakdownCards rows={rows} query={query} totalTokens={totalTokens} onDrill={onDrill} />
    );
  }
  return (
    <UsageBreakdownTable
      rows={rows}
      query={query}
      totalTokens={totalTokens}
      onSort={onSort}
      onDrill={onDrill}
    />
  );
}

function Pagination({
  offset,
  limit,
  total,
  onOffset,
}: {
  offset: number;
  limit: number;
  total: number;
  onOffset: (offset: number) => void;
}) {
  const { t } = useTranslation();
  if (total === 0) return null;
  const start = offset + 1;
  const end = Math.min(offset + limit, total);
  return (
    <div className="flex items-center justify-between gap-2">
      <span className="text-xs text-muted-foreground" data-testid="usage-breakdown-page-label">
        {t("usage:breakdownPageRange", { start, end, total })}
      </span>
      <div className="flex gap-2">
        <Button
          variant="outline"
          size="sm"
          disabled={offset <= 0}
          onClick={() => onOffset(Math.max(0, offset - limit))}
          className="cursor-pointer"
        >
          {t("usage:breakdownPrevious")}
        </Button>
        <Button
          variant="outline"
          size="sm"
          disabled={end >= total}
          onClick={() => onOffset(offset + limit)}
          className="cursor-pointer"
        >
          {t("usage:breakdownNext")}
        </Button>
      </div>
    </div>
  );
}

/** The Breakdown tab: grouped ledger rows with filters, sort, and drill-down. */
export function UsageBreakdownTab({
  range,
  refreshToken,
}: {
  range: ProviderUsageRange;
  refreshToken: number;
}) {
  const { isMobile } = useResponsiveBreakpoint();
  const { data, loading, error, query, setQuery } = useUsageBreakdown(
    range,
    undefined,
    refreshToken,
  );
  const handleSort = (column: UsageBreakdownSort) => setQuery(nextSort(query, column));
  const handleDrill = (row: UsageBreakdownRow) => setQuery(drillDown(query, row));

  return (
    <div className="space-y-4" data-testid="usage-breakdown">
      <UsageBreakdownToolbar
        query={query}
        setQuery={setQuery}
        facets={data?.facets}
        isMobile={isMobile}
      />
      <UsageBreakdownFilters query={query} onChange={(next) => setQuery(next)} />
      {data && <SummaryChips data={data} />}
      <BreakdownResults
        data={data}
        loading={loading}
        error={error}
        isMobile={isMobile}
        query={query}
        onSort={handleSort}
        onDrill={handleDrill}
      />
      {data && (
        <Pagination
          offset={query.offset ?? 0}
          limit={query.limit ?? DEFAULT_LIMIT}
          total={data.total_rows}
          onOffset={(offset) => setQuery({ offset })}
        />
      )}
    </div>
  );
}
