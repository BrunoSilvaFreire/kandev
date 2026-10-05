/**
 * Shared view filter and grouping model.
 *
 * One clause shape, one dimension registry, one filter engine, and one grouping
 * engine serve Kanban/Pipeline, List, Threads, and the sidebar task navigation.
 * Each view declares the dimensions and group keys it supports and keeps its own
 * rendering. See `docs/decisions/2026-09-29-shared-home-view-model.md`.
 */

/** One filter predicate. String-comparable value; arrays OR together within a clause. */
export type ViewFilterOp = "is" | "is_not" | "in" | "not_in" | "matches" | "not_matches";

export type ViewFilterValue = string | string[] | boolean;

/**
 * The shared clause shape. The dimension is generic so each view keeps its own
 * dimension union while sharing the shape; the sidebar and Threads types are
 * aliases of this.
 */
export type ViewFilterClause<D extends string = string> = {
  id: string;
  dimension: D;
  op: ViewFilterOp;
  value: ViewFilterValue;
};

/** The Home views that consume the shared model. */
export type HomeViewId = "kanban" | "list" | "threads" | "sidebar";

/** Grouping keys the shared model understands. A view supports a subset. */
export type ViewGroupKey =
  | "none"
  | "repository"
  | "repositoryGroup"
  | "workflow"
  | "workflowStep"
  | "executorType"
  | "state"
  | "priority";

export type ViewGroup<Item> = {
  key: string;
  label: string;
  items: Item[];
};

export type GroupedViewItems<Item> = {
  groups: ViewGroup<Item>[];
  groupKey: ViewGroupKey;
};
