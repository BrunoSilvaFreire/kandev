"use client";

import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { useKanbanDisplaySettings } from "@/hooks/use-kanban-display-settings";
import { useHomeQuickFilters } from "@/hooks/use-home-quick-filters";
import { useRepositoryGroups } from "@/hooks/use-repository-groups";
import { QuickFilterBar, type QuickFilterAdapter } from "@/components/view-model/quick-filter-bar";
import { ViewGroupControl } from "@/components/view-model/view-group-control";
import { getViewDimensionMeta } from "@/lib/view-model/dimensions";
import { SUPPORTED_GROUPS } from "@/lib/view-model/views";
import type { ViewGroupKey } from "@/lib/view-model/types";
import type { KanbanFilterDimension } from "@/lib/view-model/kanban";
import { getOpLabel } from "@/components/task/sidebar-filter/filter-dimension-registry";
import { TASK_PRIORITY_TOKENS, TASK_PRIORITY_LABEL_KEYS } from "@/lib/tasks/task-priority";
import { TASK_STATE_ORDER } from "@/lib/tasks/tasks-list-options";

// `labelKey` values are catalog keys, not copy: module scope, so a resolved
// `t()` here would freeze at the boot locale.
const DIMENSION_LABEL_KEYS: Record<KanbanFilterDimension, string> = {
  workflow: "task:filterDimensionWorkflow",
  workflowStep: "task:filterDimensionWorkflowStep",
  state: "task:filterDimensionState",
  repository: "task:filterDimensionRepository",
  repositoryGroup: "task:filterDimensionRepositoryGroup",
  priority: "task:filterDimensionPriority",
};

const TASK_STATE_LABEL_KEYS: Record<string, string> = {
  CREATED: "task:statusTodo",
  SCHEDULING: "task:statusTodo",
  TODO: "task:statusTodo",
  IN_PROGRESS: "task:statusInProgress",
  WAITING_FOR_INPUT: "task:statusInProgress",
  BLOCKED: "task:statusBlocked",
  REVIEW: "task:statusInReview",
  COMPLETED: "task:statusCompleted",
  FAILED: "task:filterStateReview",
  CANCELLED: "task:statusCancelled",
};

/**
 * The shared Quick Filter bar and grouping picker for the Kanban and List Home
 * views. Both consume the same persisted `filters`/`group` arrays through the
 * shared view model; only the workflow/step selector stays view-structural.
 */
export function KanbanViewFilters() {
  const { t } = useTranslation();
  const display = useKanbanDisplaySettings();
  const repositoryGroups = useRepositoryGroups();
  const view = display.effectiveTaskListingView === "list" ? "list" : "kanban";
  const { dimensions } = useHomeQuickFilters(view);
  const options = useMemo(
    () => ({
      workflow: display.workflows.map((workflow) => ({
        value: workflow.id,
        label: workflow.name,
      })),
      state: TASK_STATE_ORDER.map((state) => ({
        value: state,
        label: TASK_STATE_LABEL_KEYS[state] ? t(TASK_STATE_LABEL_KEYS[state]) : state,
      })),
      repository: display.repositories.map((repo) => ({ value: repo.id, label: repo.name })),
      repositoryGroup: repositoryGroups.map((group) => ({ value: group.id, label: group.name })),
      priority: TASK_PRIORITY_TOKENS.map((token) => ({
        value: token,
        label: t(TASK_PRIORITY_LABEL_KEYS[token]),
      })),
      workflowStep: [],
    }),
    [display.repositories, display.workflows, repositoryGroups, t],
  );

  const adapter: QuickFilterAdapter = {
    clauses: display.filters,
    dimensions,
    getMeta: (dimension) => ({ dimension, ...getViewDimensionMeta(dimension) }),
    getDimensionLabel: (dimension) => t(DIMENSION_LABEL_KEYS[dimension as KanbanFilterDimension]),
    getOpLabel: (op, valueKind) => getOpLabel(op, valueKind),
    optionsForDimension: (dimension) => options[dimension as KanbanFilterDimension] ?? [],
    createClause: (dimension) => {
      const meta = getViewDimensionMeta(dimension);
      return {
        id: `quick-${view}-${dimension}`,
        dimension,
        op: meta.defaultOp,
        value: meta.defaultValue,
      };
    },
    setClause: (clause) => {
      const exists = display.filters.some((filter) => filter.dimension === clause.dimension);
      display.onFiltersChange(
        exists
          ? display.filters.map((filter) =>
              filter.dimension === clause.dimension ? clause : filter,
            )
          : [...display.filters, clause],
      );
    },
    removeClause: (id) =>
      display.onFiltersChange(display.filters.filter((filter) => filter.id !== id)),
  };

  // The List filters its loaded page through the same persisted clauses; its
  // grouping stays in its own server-backed control, so no group picker here.
  return (
    <div className="flex flex-wrap items-center gap-2" data-testid="kanban-view-filters">
      <QuickFilterBar adapter={adapter} testId={`${view}-quick-filters`} />
      {view === "kanban" ? (
        <div className="hidden sm:block">
          <ViewGroupControl
            value={display.group}
            groups={SUPPORTED_GROUPS[view]}
            onChange={(next: ViewGroupKey) => display.onGroupChange(next)}
            testId={`${view}-group-select`}
          />
        </div>
      ) : null}
    </div>
  );
}
