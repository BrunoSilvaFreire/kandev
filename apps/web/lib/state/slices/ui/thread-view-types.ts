import type {
  ViewFilterClause,
  ViewFilterOp,
  ViewFilterValue,
  ViewGroupKey,
} from "@/lib/view-model/types";

export type ThreadTaskScope =
  | { mode: "all"; taskIds: [] }
  | { mode: "selected"; taskIds: string[] };

export type ThreadFilterDimension =
  | "threadStatus"
  | "pendingAction"
  | "taskState"
  | "workflow"
  | "workflowStep"
  | "repository"
  | "repositoryGroup"
  | "primaryAgent"
  | "executorType"
  | "priority"
  | "blocked"
  | "hasQueuedPrompts"
  | "hasActiveSubagents"
  | "hasDiff"
  | "hasPR"
  | "prNeedsAttention"
  | "taskType"
  | "titleMatch"
  | "hasActiveError"
  | "taskLabel"
  | "taskOrigin"
  | "hasMultipleSessions";

export type ThreadFilterOp = ViewFilterOp;
export type ThreadFilterValue = ViewFilterValue;

export type ThreadFilterClause = ViewFilterClause<ThreadFilterDimension>;

/** Group keys a Threads view can render. `none` is always available. */
export type ThreadGroupKey = Extract<
  ViewGroupKey,
  "none" | "repository" | "repositoryGroup" | "workflow" | "state" | "priority"
>;

export type ThreadSortKey =
  | "attention"
  | "lastActivityAt"
  | "updatedAt"
  | "createdAt"
  | "title"
  | "taskState"
  | "workflow"
  | "priority"
  | "primaryAgent";
export type ThreadSortDirection = "asc" | "desc";
export type ThreadSortSpec = { key: ThreadSortKey; direction: ThreadSortDirection };
export type ThreadLayout = "columns" | "grid";

export type ThreadView = {
  id: string;
  name: string;
  taskScope: ThreadTaskScope;
  filters: ThreadFilterClause[];
  sort: ThreadSortSpec;
  group: ThreadGroupKey;
  maxColumns: number | null;
  layout: ThreadLayout;
  autoHideComposer: boolean;
};

export type ThreadViewDraft = Omit<ThreadView, "id" | "name"> & {
  baseViewId: string;
};

/** A complete backend-owned Threads view projection kept during a local write. */
export type ThreadViewSnapshot = {
  views: ThreadView[];
  activeViewId: string;
  draft: ThreadViewDraft | null;
};

export type ThreadViewSliceState = {
  views: ThreadView[];
  activeViewId: string;
  draft: ThreadViewDraft | null;
  syncError: string | null;
  /** Keep server echoes from replacing optimistic edits until the latest write settles. */
  syncPending: boolean;
  /** Newer authoritative settings received while an optimistic write is pending. */
  deferredServerState: ThreadViewSnapshot | null;
  orderResetGeneration: number;
};
