import type { HomeViewId, ViewGroupKey } from "./types";

/**
 * Dimensions each Home view offers. Values are dimension ids; keeping this as a
 * string list lets each view keep its own typed union while the shared model
 * answers "does this view support this dimension?" in one place.
 */
export const SUPPORTED_DIMENSIONS: Record<HomeViewId, readonly string[]> = {
  sidebar: [
    "archived",
    "state",
    "workflow",
    "workflowStep",
    "executorType",
    "repository",
    "repositoryGroup",
    "hasDiff",
    "hasPR",
    "isPRReview",
    "isIssueWatch",
    "titleMatch",
  ],
  threads: [
    "threadStatus",
    "pendingAction",
    "taskState",
    "workflow",
    "workflowStep",
    "repository",
    "repositoryGroup",
    "primaryAgent",
    "executorType",
    "priority",
    "blocked",
    "hasQueuedPrompts",
    "hasActiveSubagents",
    "hasDiff",
    "hasPR",
    "prNeedsAttention",
    "taskType",
    "titleMatch",
    "hasActiveError",
    "taskLabel",
    "taskOrigin",
    "hasMultipleSessions",
  ],
  kanban: ["workflow", "state", "repository", "repositoryGroup", "priority"],
  list: ["workflow", "state", "repository", "repositoryGroup", "priority"],
};

/** Group keys each Home view can render. `none` is always available. */
export const SUPPORTED_GROUPS: Record<HomeViewId, readonly ViewGroupKey[]> = {
  sidebar: [
    "none",
    "repository",
    "repositoryGroup",
    "workflow",
    "workflowStep",
    "executorType",
    "state",
  ],
  threads: ["none", "repository", "repositoryGroup", "workflow", "state", "priority"],
  kanban: ["none", "repository", "repositoryGroup", "workflow", "state", "priority"],
  list: ["none", "repository", "repositoryGroup", "workflow", "state", "priority"],
};

export function viewSupportsDimension(view: HomeViewId, dimension: string): boolean {
  return SUPPORTED_DIMENSIONS[view].includes(dimension);
}

export function viewSupportsGroup(view: HomeViewId, groupKey: ViewGroupKey): boolean {
  return SUPPORTED_GROUPS[view].includes(groupKey);
}
