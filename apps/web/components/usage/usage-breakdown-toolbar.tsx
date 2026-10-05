"use client";

import { useState } from "react";
import { Button } from "@kandev/ui/button";
import { Drawer, DrawerContent, DrawerHeader, DrawerTitle, DrawerTrigger } from "@kandev/ui/drawer";
import { Input } from "@kandev/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { ToggleGroup, ToggleGroupItem } from "@kandev/ui/toggle-group";
import { IconFilter } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import type {
  UsageBreakdownFacets,
  UsageBreakdownGroupBy,
  UsageBreakdownQuery,
  UsageBreakdownSort,
} from "@/lib/types/provider-usage";

const ALL = "__all__";

const GROUP_KEYS: Record<UsageBreakdownGroupBy, string> = {
  session: "usage:breakdownGroupSession",
  task: "usage:breakdownGroupTask",
  model: "usage:breakdownGroupModel",
  provider: "usage:breakdownGroupProvider",
  agent: "usage:breakdownGroupAgent",
  day: "usage:breakdownGroupDay",
};

const SORT_KEYS: Record<UsageBreakdownSort, string> = {
  tokens: "usage:breakdownSortTokens",
  cost: "usage:breakdownSortCost",
  events: "usage:breakdownSortEvents",
  last_at: "usage:breakdownSortLastAt",
};

const GROUPS = Object.keys(GROUP_KEYS) as UsageBreakdownGroupBy[];
const SORTS = Object.keys(SORT_KEYS) as UsageBreakdownSort[];

export type BreakdownFilterPatch = Partial<UsageBreakdownQuery>;

/** A single-select filter whose "all" option clears the filter. */
function FacetSelect({
  label,
  value,
  options,
  onChange,
  testId,
}: {
  label: string;
  value: string | undefined;
  options: string[];
  onChange: (value: string | undefined) => void;
  testId: string;
}) {
  const { t } = useTranslation();
  const choices = options.filter((option) => option !== "");
  if (choices.length === 0) return null;
  return (
    <Select
      value={value ?? ALL}
      onValueChange={(next) => onChange(next === ALL ? undefined : next)}
    >
      <SelectTrigger size="sm" data-testid={testId} aria-label={label} className="w-40">
        <SelectValue placeholder={label} />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value={ALL}>{t("usage:breakdownFilterAll", { label })}</SelectItem>
        {choices.map((option) => (
          <SelectItem key={option} value={option}>
            {option}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

function GroupToggle({
  query,
  setQuery,
}: {
  query: UsageBreakdownQuery;
  setQuery: (patch: BreakdownFilterPatch) => void;
}) {
  const { t } = useTranslation();
  return (
    <ToggleGroup
      type="single"
      variant="outline"
      size="sm"
      value={query.groupBy ?? "session"}
      onValueChange={(value) => value && setQuery({ groupBy: value as UsageBreakdownGroupBy })}
      className="max-w-full overflow-x-auto"
      data-testid="usage-breakdown-groups"
    >
      {GROUPS.map((group) => (
        <ToggleGroupItem key={group} value={group} className="cursor-pointer whitespace-nowrap">
          {t(GROUP_KEYS[group])}
        </ToggleGroupItem>
      ))}
    </ToggleGroup>
  );
}

function SearchInput({
  query,
  setQuery,
}: {
  query: UsageBreakdownQuery;
  setQuery: (patch: BreakdownFilterPatch) => void;
}) {
  const { t } = useTranslation();
  return (
    <Input
      value={query.q ?? ""}
      onChange={(event) => setQuery({ q: event.target.value })}
      placeholder={t("usage:breakdownSearchPlaceholder")}
      aria-label={t("usage:breakdownSearchPlaceholder")}
      data-testid="usage-breakdown-search"
      className="w-full sm:w-56"
    />
  );
}

function FilterFields({
  query,
  setQuery,
  facets,
}: {
  query: UsageBreakdownQuery;
  setQuery: (patch: BreakdownFilterPatch) => void;
  facets?: UsageBreakdownFacets;
}) {
  const { t } = useTranslation();
  return (
    <>
      <FacetSelect
        label={t("usage:breakdownFilterProvider")}
        value={query.provider}
        options={facets?.providers ?? []}
        onChange={(value) => setQuery({ provider: value })}
        testId="usage-breakdown-provider"
      />
      <FacetSelect
        label={t("usage:breakdownFilterModel")}
        value={query.model}
        options={facets?.models ?? []}
        onChange={(value) => setQuery({ model: value })}
        testId="usage-breakdown-model"
      />
      <FacetSelect
        label={t("usage:breakdownFilterAgent")}
        value={query.agentType}
        options={facets?.agent_types ?? []}
        onChange={(value) => setQuery({ agentType: value })}
        testId="usage-breakdown-agent"
      />
    </>
  );
}

function MobileFilters({
  query,
  setQuery,
  facets,
}: {
  query: UsageBreakdownQuery;
  setQuery: (patch: BreakdownFilterPatch) => void;
  facets?: UsageBreakdownFacets;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  return (
    <Drawer open={open} onOpenChange={setOpen}>
      <DrawerTrigger asChild>
        <Button
          variant="outline"
          size="sm"
          className="h-11 cursor-pointer"
          data-testid="usage-breakdown-filters-button"
        >
          <IconFilter className="mr-1 h-4 w-4" />
          {t("usage:breakdownFilters")}
        </Button>
      </DrawerTrigger>
      <DrawerContent>
        <DrawerHeader>
          <DrawerTitle>{t("usage:breakdownFilters")}</DrawerTitle>
        </DrawerHeader>
        <div className="flex flex-col gap-3 overflow-y-auto p-4 pb-[max(1rem,env(safe-area-inset-bottom))]">
          <FilterFields query={query} setQuery={setQuery} facets={facets} />
          <Select
            value={query.sort ?? "tokens"}
            onValueChange={(value) => setQuery({ sort: value as UsageBreakdownSort })}
          >
            <SelectTrigger size="sm" aria-label={t("usage:breakdownSortLabel")} className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {SORTS.map((sort) => (
                <SelectItem key={sort} value={sort}>
                  {t(SORT_KEYS[sort])}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Button
            variant="secondary"
            className="h-11 cursor-pointer"
            onClick={() => setOpen(false)}
          >
            {t("usage:breakdownDone")}
          </Button>
        </div>
      </DrawerContent>
    </Drawer>
  );
}

/** Group, search, and facet controls, composed differently per viewport. */
export function UsageBreakdownToolbar({
  query,
  setQuery,
  facets,
  isMobile,
}: {
  query: UsageBreakdownQuery;
  setQuery: (patch: BreakdownFilterPatch) => void;
  facets?: UsageBreakdownFacets;
  isMobile: boolean;
}) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      <GroupToggle query={query} setQuery={setQuery} />
      <SearchInput query={query} setQuery={setQuery} />
      {!isMobile && <FilterFields query={query} setQuery={setQuery} facets={facets} />}
      {isMobile && <MobileFilters query={query} setQuery={setQuery} facets={facets} />}
    </div>
  );
}
