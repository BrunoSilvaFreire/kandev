/**
 * The shared dimension registry for the Home view model.
 *
 * One table holds the substantive metadata for every filter dimension that any
 * Home view offers: its value kind, the operators it accepts, its default
 * operator and value, an optional placeholder, and any fixed enum options.
 * Views keep only their display copy (a catalog `labelKey`), so the sidebar and
 * Threads registries are thin projections of this table rather than parallel
 * definitions.
 *
 * `labelKey` / `placeholderKey` hold catalog keys, not resolved copy: this is
 * module scope, so a resolved `t()` here would freeze at the boot locale.
 */

import type { ViewFilterOp, ViewFilterValue } from "./types";

export type ViewDimensionValueKind = "boolean" | "enum" | "text";

export type ViewFixedOption = { value: string; labelKey: string };

export type ViewDimensionMeta = {
  valueKind: ViewDimensionValueKind;
  ops: readonly ViewFilterOp[];
  defaultOp: ViewFilterOp;
  defaultValue: ViewFilterValue;
  placeholderKey?: string;
  fixedOptions?: readonly ViewFixedOption[];
};

const BOOLEAN_IS: ViewDimensionMeta = {
  valueKind: "boolean",
  ops: ["is", "is_not"],
  defaultOp: "is",
  defaultValue: true,
};

const ENUM_IN: ViewDimensionMeta = {
  valueKind: "enum",
  ops: ["is", "is_not", "in", "not_in"],
  defaultOp: "is",
  defaultValue: "",
};

const TEXT: ViewDimensionMeta = {
  valueKind: "text",
  ops: ["matches", "not_matches"],
  defaultOp: "matches",
  defaultValue: "",
  placeholderKey: "task:filterTitlePlaceholder",
};

/**
 * Every dimension any Home view can filter on, keyed by dimension id. A view
 * exposes a subset; the payload is the same wherever the id appears, so
 * `repository` behaves identically on Kanban, List, Threads and the sidebar.
 */
export const VIEW_DIMENSION_METAS: Record<string, ViewDimensionMeta> = {
  // Sidebar task navigation.
  archived: BOOLEAN_IS,
  state: {
    valueKind: "enum",
    ops: ["in", "not_in", "is", "is_not"],
    defaultOp: "in",
    defaultValue: ["review", "in_progress"],
    fixedOptions: [
      { value: "review", labelKey: "task:filterStateReview" },
      { value: "in_progress", labelKey: "task:filterStateInProgress" },
      { value: "backlog", labelKey: "task:filterStateBacklog" },
    ],
  },
  isPRReview: BOOLEAN_IS,
  isIssueWatch: BOOLEAN_IS,
  // Shared by both views.
  hasDiff: BOOLEAN_IS,
  hasPR: BOOLEAN_IS,
  workflow: ENUM_IN,
  workflowStep: ENUM_IN,
  executorType: ENUM_IN,
  repository: ENUM_IN,
  repositoryGroup: ENUM_IN,
  titleMatch: TEXT,
  priority: { ...ENUM_IN, defaultValue: "medium" },
  // Threads-only dimensions.
  threadStatus: {
    valueKind: "enum",
    ops: ["is", "is_not", "in", "not_in"],
    defaultOp: "is",
    defaultValue: "needs_action",
    fixedOptions: [
      { value: "needs_action", labelKey: "threads:statusNeedsAction" },
      { value: "running", labelKey: "threads:statusRunning" },
      { value: "waiting", labelKey: "threads:statusWaiting" },
      { value: "ready_for_review", labelKey: "threads:statusReadyForReview" },
    ],
  },
  pendingAction: {
    valueKind: "enum",
    ops: ["is", "is_not", "in", "not_in"],
    defaultOp: "is",
    defaultValue: "clarification",
    fixedOptions: [
      { value: "clarification", labelKey: "threads:pendingClarification" },
      { value: "permission", labelKey: "threads:pendingPermission" },
      { value: "none", labelKey: "threads:pendingNone" },
    ],
  },
  taskState: { ...ENUM_IN, defaultValue: "IN_PROGRESS" },
  taskType: {
    valueKind: "enum",
    ops: ["is", "is_not", "in", "not_in"],
    defaultOp: "is",
    defaultValue: "standard",
    fixedOptions: [
      { value: "standard", labelKey: "threads:taskTypeStandard" },
      { value: "pull_request_review", labelKey: "threads:taskTypePullRequestReview" },
      { value: "issue_watch", labelKey: "threads:taskTypeIssueWatch" },
    ],
  },
  primaryAgent: ENUM_IN,
  blocked: BOOLEAN_IS,
  hasQueuedPrompts: BOOLEAN_IS,
  hasActiveSubagents: BOOLEAN_IS,
  prNeedsAttention: BOOLEAN_IS,
  hasActiveError: BOOLEAN_IS,
  taskLabel: ENUM_IN,
  taskOrigin: { ...ENUM_IN, defaultValue: "manual" },
  hasMultipleSessions: BOOLEAN_IS,
};

/** The metadata for a dimension id; throws rather than silently filtering it out. */
export function getViewDimensionMeta(dimension: string): ViewDimensionMeta {
  const meta = VIEW_DIMENSION_METAS[dimension];
  if (!meta) throw new Error(`Unknown view dimension: ${dimension}`);
  return meta;
}

/** All dimension ids the shared registry knows, in declaration order. */
export const VIEW_DIMENSION_IDS: readonly string[] = Object.keys(VIEW_DIMENSION_METAS);
