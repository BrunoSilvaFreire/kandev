"use client";

import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { IconPlus } from "@tabler/icons-react";
import type { ViewFilterClause, ViewFilterOp, ViewFilterValue } from "@/lib/view-model/types";
import {
  ViewFilterEditor,
  type ViewFilterEditorMeta,
  type ViewFilterEditorOption,
} from "./view-filter-editor";

/**
 * Everything the shared Quick Filter bar needs from one Home view. Views own
 * their persisted `filters` array; the bar only reads it and writes single
 * clauses through these callbacks, so the same component serves Kanban, List,
 * Threads and the sidebar.
 */
export type QuickFilterAdapter = {
  /** The view's active clauses (persisted, not a copy). */
  clauses: ViewFilterClause[];
  /** Ordered dimension ids the user pinned as quick filters. */
  dimensions: string[];
  getMeta: (dimension: string) => ViewFilterEditorMeta;
  getDimensionLabel: (dimension: string) => string;
  getOpLabel: (op: ViewFilterOp, valueKind: ViewFilterEditorMeta["valueKind"]) => string;
  optionsForDimension: (dimension: string) => ViewFilterEditorOption[];
  /** Build the default clause used when the dimension is not yet filtered. */
  createClause: (dimension: string) => ViewFilterClause;
  /** Insert or replace the clause with the same dimension. */
  setClause: (clause: ViewFilterClause) => void;
  removeClause: (id: string) => void;
};

/**
 * The persistent, always-visible filter controls for a Home view. It renders
 * one shared `ViewFilterEditor` row per pinned dimension and an "Add" chip for
 * a pinned dimension that has no active clause yet.
 */
export function QuickFilterBar({
  adapter,
  testId,
}: {
  adapter: QuickFilterAdapter;
  testId: string;
}) {
  const { t } = useTranslation();
  if (adapter.dimensions.length === 0) return null;

  return (
    <div className="flex flex-wrap items-center gap-1.5" data-testid={testId}>
      {adapter.dimensions.map((dimension) => {
        const clause = adapter.clauses.find((candidate) => candidate.dimension === dimension);
        if (clause) {
          return (
            <ViewFilterEditor
              key={dimension}
              clause={clause}
              dimensions={[adapter.getMeta(dimension)]}
              getMeta={adapter.getMeta}
              getDimensionLabel={adapter.getDimensionLabel}
              getOpLabel={adapter.getOpLabel}
              optionsForDimension={adapter.optionsForDimension}
              onChange={adapter.setClause}
              onRemove={() => adapter.removeClause(clause.id)}
            />
          );
        }
        return (
          <Button
            key={dimension}
            type="button"
            variant="outline"
            size="sm"
            className="h-6 cursor-pointer text-xs"
            onClick={() => adapter.setClause(adapter.createClause(dimension))}
            data-testid={`${testId}-add-${dimension}`}
          >
            <IconPlus className="mr-1 h-3 w-3" />
            {adapter.getDimensionLabel(dimension)}
          </Button>
        );
      })}
      <span className="sr-only">{t("task:quickFilters")}</span>
    </div>
  );
}

/** Read a dimension's current scalar value from a clause list. */
export function quickFilterValueFor(
  clauses: ViewFilterClause[],
  dimension: string,
): ViewFilterValue | undefined {
  return clauses.find((clause) => clause.dimension === dimension)?.value;
}
