"use client";

import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { selectSidebarViews } from "@/lib/state/slices/ui/sidebar-workspace-state";
import { QuickFilterBar, type QuickFilterAdapter } from "@/components/view-model/quick-filter-bar";
import { useHomeQuickFilters } from "@/hooks/use-home-quick-filters";
import type { FilterClause, FilterDimension } from "@/lib/state/slices/ui/sidebar-view-types";
import { getDimensionEnumOptions, getDimensionMeta, getOpLabel } from "./filter-dimension-registry";
import { createSidebarViewEditorCurrent } from "./sidebar-view-editor";
import {
  executorTypeOptions,
  repositoryOptions,
  workflowOptions,
  workflowStepOptions,
} from "./use-filter-value-options";

/**
 * The sidebar's pinned Quick Filters. Reads and writes the active saved view's
 * persisted `filters` through the shared model, exactly like the full editor
 * nested behind the gear, but always visible. Horizontally scrollable because
 * the sidebar is narrower than a row of editor controls.
 */
export function SidebarQuickFilters() {
  const { t, i18n } = useTranslation();
  const activeViewId = useAppStore((s) => selectSidebarViews(s).activeViewId);
  const views = useAppStore((s) => selectSidebarViews(s).views);
  const draft = useAppStore((s) => selectSidebarViews(s).draft);
  const updateDraft = useAppStore((s) => s.updateSidebarDraft);
  const snapshots = useAppStore((s) => s.kanbanMulti.snapshots);
  const repositoriesByWorkspace = useAppStore((s) => s.repositories.itemsByWorkspaceId);
  const repositorySets = useAppStore(
    (s) => s.repositorySets.itemsByWorkspaceId[s.workspaces.activeId ?? ""],
  );
  const { dimensions } = useHomeQuickFilters("sidebar");

  const activeView = views.find((view) => view.id === activeViewId);
  const current = createSidebarViewEditorCurrent(activeView, draft);

  const optionsByDimension = useMemo<
    Record<string, Array<{ value: string; label: string; color?: string; group?: string }>>
  >(
    () => ({
      workflow: workflowOptions(snapshots),
      workflowStep: workflowStepOptions(snapshots),
      executorType: executorTypeOptions(snapshots),
      repository: repositoryOptions(repositoriesByWorkspace),
      repositoryGroup: (repositorySets ?? []).map((set) => ({
        value: set.id,
        label: set.name,
      })),
    }),
    [snapshots, repositoriesByWorkspace, repositorySets, i18n.language],
  );

  if (dimensions.length === 0) return null;

  const adapter: QuickFilterAdapter = {
    clauses: current.filters,
    dimensions,
    getMeta: (dimension) => getDimensionMeta(dimension as FilterDimension),
    getDimensionLabel: (dimension) => t(getDimensionMeta(dimension as FilterDimension).labelKey),
    getOpLabel: (op, valueKind) => getOpLabel(op, valueKind),
    optionsForDimension: (dimension) => {
      const meta = getDimensionMeta(dimension as FilterDimension);
      return getDimensionEnumOptions(meta) ?? optionsByDimension[dimension] ?? [];
    },
    createClause: (dimension) => {
      const meta = getDimensionMeta(dimension as FilterDimension);
      return {
        id: `sidebar-quick-${dimension}`,
        dimension: dimension as FilterDimension,
        op: meta.defaultOp,
        value: meta.defaultValue,
      };
    },
    setClause: (clause) => {
      const next = clause as FilterClause;
      const exists = current.filters.some((filter) => filter.dimension === next.dimension);
      updateDraft({
        filters: exists
          ? current.filters.map((filter) => (filter.dimension === next.dimension ? next : filter))
          : [...current.filters, next],
      });
    },
    removeClause: (id) =>
      updateDraft({ filters: current.filters.filter((filter) => filter.id !== id) }),
  };

  return (
    <div
      className="overflow-x-auto border-b border-border/60 px-2 py-1 md:px-3"
      data-testid="sidebar-quick-filters-row"
    >
      <QuickFilterBar adapter={adapter} testId="sidebar-quick-filters" />
    </div>
  );
}
