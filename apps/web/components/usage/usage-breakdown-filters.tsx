"use client";

import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { IconX } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import type { UsageBreakdownQuery } from "@/lib/types/provider-usage";
import {
  activeFilterChips,
  clearFilters,
  removeFilter,
  type BreakdownFilterKey,
} from "./usage-breakdown-model";

const FILTER_LABEL_KEYS: Record<BreakdownFilterKey, string> = {
  provider: "usage:breakdownFilterProvider",
  model: "usage:breakdownFilterModel",
  agentType: "usage:breakdownFilterAgent",
  taskId: "usage:breakdownFilterTask",
  sessionId: "usage:breakdownFilterSession",
  q: "usage:breakdownFilterSearch",
};

/** Removable chips for the active drill-down and toolbar filters. */
export function UsageBreakdownFilters({
  query,
  onChange,
}: {
  query: UsageBreakdownQuery;
  onChange: (next: UsageBreakdownQuery) => void;
}) {
  const { t } = useTranslation();
  const chips = activeFilterChips(query);
  if (chips.length === 0) return null;
  return (
    <div className="flex flex-wrap items-center gap-2" data-testid="usage-breakdown-filters">
      {chips.map((chip) => {
        const name = t(FILTER_LABEL_KEYS[chip.key]);
        return (
          <Badge key={chip.key} variant="secondary" className="gap-1 text-xs">
            <span className="text-muted-foreground">{name}</span>
            <span className="max-w-[12rem] truncate">{chip.value}</span>
            <button
              type="button"
              aria-label={t("usage:breakdownRemoveFilter", { name })}
              onClick={() => onChange(removeFilter(query, chip.key))}
              className="cursor-pointer"
            >
              <IconX className="h-3 w-3" />
            </button>
          </Badge>
        );
      })}
      <Button
        variant="ghost"
        size="sm"
        className="h-6 cursor-pointer px-2 text-xs"
        onClick={() => onChange(clearFilters(query))}
      >
        {t("usage:breakdownClearFilters")}
      </Button>
    </div>
  );
}
