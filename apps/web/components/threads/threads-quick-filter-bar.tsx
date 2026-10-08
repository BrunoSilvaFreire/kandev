"use client";

import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { QuickFilterBar, type QuickFilterAdapter } from "@/components/view-model/quick-filter-bar";
import { ViewGroupControl } from "@/components/view-model/view-group-control";
import { SUPPORTED_GROUPS } from "@/lib/view-model/views";
import type { ViewGroupKey } from "@/lib/view-model/types";
import { useHomeQuickFilters } from "@/hooks/use-home-quick-filters";
import { useRepositoryGroups } from "@/hooks/use-repository-groups";
import type { ThreadCandidate } from "@/lib/threads/thread-view-query";
import type {
  ThreadFilterClause,
  ThreadFilterDimension,
  ThreadView,
  ThreadViewDraft,
} from "@/lib/state/slices/ui/thread-view-types";
import type { Repository } from "@/lib/types/http";
import {
  getThreadDimensionLabel,
  getThreadDimensionMeta,
  getThreadFilterOpLabel,
  getThreadFilterOptions,
} from "./threads-view-filter-registry";

/** The Threads view's pinned Quick Filters, writing its persisted `filters`. */
export function ThreadsQuickFilterBar({
  activeView,
  draft,
  candidates,
  repositories,
}: {
  activeView: ThreadView;
  draft: ThreadViewDraft | null;
  candidates: ThreadCandidate[];
  repositories: ReadonlyArray<Pick<Repository, "id" | "name">>;
}) {
  const { t } = useTranslation();
  const updateDraft = useAppStore((state) => state.updateThreadViewDraft);
  const { dimensions } = useHomeQuickFilters("threads");
  const repositoryGroups = useRepositoryGroups();
  const repositoryNames = useMemo(
    () => new Map(repositories.map((repo) => [String(repo.id), repo.name])),
    [repositories],
  );
  const current = draft && draft.baseViewId === activeView.id ? draft : activeView;
  const adapter: QuickFilterAdapter = {
    clauses: current.filters,
    dimensions,
    getMeta: (dimension) => getThreadDimensionMeta(dimension as ThreadFilterDimension),
    getDimensionLabel: (dimension) =>
      getThreadDimensionLabel(dimension as ThreadFilterDimension, t),
    getOpLabel: (op) => getThreadFilterOpLabel(op, t),
    optionsForDimension: (dimension) =>
      getThreadFilterOptions(
        dimension as ThreadFilterDimension,
        candidates,
        t,
        repositoryNames,
        repositoryGroups,
      ),
    createClause: (dimension) => {
      const meta = getThreadDimensionMeta(dimension as ThreadFilterDimension);
      return {
        id: `thread-quick-${dimension}`,
        dimension: dimension as ThreadFilterDimension,
        op: meta.defaultOp,
        value: meta.defaultValue,
      };
    },
    setClause: (clause) => {
      const nextClause = clause as ThreadFilterClause;
      const exists = current.filters.some((filter) => filter.dimension === clause.dimension);
      updateDraft({
        filters: exists
          ? current.filters.map((filter) =>
              filter.dimension === clause.dimension ? nextClause : filter,
            )
          : [...current.filters, nextClause],
      });
    },
    removeClause: (id) =>
      updateDraft({ filters: current.filters.filter((filter) => filter.id !== id) }),
  };
  return (
    <div className="flex flex-wrap items-center gap-2">
      <QuickFilterBar adapter={adapter} testId="threads-quick-filters" />
      <ViewGroupControl
        value={current.group}
        groups={SUPPORTED_GROUPS.threads}
        onChange={(group: ViewGroupKey) => updateDraft({ group: group as ThreadView["group"] })}
        testId="threads-group-select"
      />
    </div>
  );
}
