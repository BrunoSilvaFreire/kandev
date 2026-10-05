/**
 * Shared dimension predicates.
 *
 * Each Home view keeps its own item shape, so the shared registry describes a
 * dimension's metadata while these accessors map a concrete item to the value
 * that dimension reads. The sidebar task accessor and the Threads candidate
 * accessor live together here so a dimension added once is evaluated the same
 * way everywhere; `applyViewFilters` consumes the result.
 */

import type { TaskSwitcherItem } from "@/components/task/task-switcher";
import type { ThreadCandidate } from "@/lib/threads/thread-view-query";
import { getStateBucket } from "@/lib/sidebar/effective-task-tree-state";
import type { RepositoryGroup } from "./repository-group";
import { matchingRepositoryGroupIds } from "./repository-group";
import type { ViewFilterValue } from "./types";

export type ViewDimensionContext = {
  /** Ordered repository groups used by the `repositoryGroup` dimension. */
  repositoryGroups?: readonly RepositoryGroup[];
};

function sidebarRepositoryGroupValue(
  task: TaskSwitcherItem,
  groups: readonly RepositoryGroup[],
): string[] {
  const ids = new Set<string>();
  for (const link of task.repositoryLinks ?? []) ids.add(String(link.repository_id));
  return matchingRepositoryGroupIds([...ids], groups);
}

/** Maps a sidebar task to the value the dimension reads. */
// eslint-disable-next-line complexity -- The dimension registry is intentionally exhaustive and type-safe.
export function sidebarTaskDimensionValue(
  task: TaskSwitcherItem,
  dimension: string,
  context?: ViewDimensionContext,
): ViewFilterValue | undefined {
  switch (dimension) {
    case "archived":
      return task.isArchived === true;
    case "state":
      return getStateBucket(task);
    case "workflow":
      return task.workflowId;
    case "workflowStep":
      return task.workflowStepId;
    case "executorType":
      return task.remoteExecutorType;
    case "repository":
      return task.repositoryPath;
    case "repositoryGroup":
      return sidebarRepositoryGroupValue(task, context?.repositoryGroups ?? []);
    case "hasDiff": {
      const ds = task.diffStats;
      return !!ds && (ds.additions > 0 || ds.deletions > 0);
    }
    case "hasPR":
      return !!task.prInfo;
    case "isPRReview":
      return task.isPRReview === true;
    case "isIssueWatch":
      return task.isIssueWatch === true;
    case "titleMatch":
      return task.title ?? "";
    default:
      return undefined;
  }
}

/** Maps a Threads candidate to the value the dimension reads. */
// eslint-disable-next-line complexity -- The dimension registry is intentionally exhaustive and type-safe.
export function threadCandidateDimensionValue(
  candidate: ThreadCandidate,
  dimension: string,
  context?: ViewDimensionContext,
): ViewFilterValue | undefined {
  switch (dimension) {
    case "threadStatus":
      return candidate.threadStatus;
    case "pendingAction":
      return candidate.pendingAction ?? candidate.taskPendingAction ?? "none";
    case "taskState":
      return candidate.taskState ?? "unknown";
    case "workflow":
      return candidate.workflowId;
    case "workflowStep":
      return candidate.workflowStepId;
    case "repository":
      return candidate.repositoryIds;
    case "repositoryGroup":
      return matchingRepositoryGroupIds(candidate.repositoryIds, context?.repositoryGroups ?? []);
    case "primaryAgent":
      return candidate.primaryAgentProfileId ?? "unknown";
    case "executorType":
      return candidate.executorType ?? "unknown";
    case "priority":
      return candidate.priority ?? "unknown";
    case "blocked":
      return candidate.blocked;
    case "hasQueuedPrompts":
      return candidate.queuedPromptCount > 0;
    case "hasActiveSubagents":
      return candidate.activeSubagentCount > 0;
    case "hasDiff":
      return candidate.hasDiff;
    case "hasPR":
      return candidate.hasPR;
    case "prNeedsAttention":
      return candidate.prNeedsAttention;
    case "taskType":
      return candidate.taskType;
    case "titleMatch":
      return candidate.title;
    case "hasActiveError":
      return candidate.hasActiveError;
    case "taskLabel":
      return candidate.labels;
    case "taskOrigin":
      return candidate.taskOrigin;
    case "hasMultipleSessions":
      return candidate.hasMultipleSessions;
    default:
      return undefined;
  }
}
