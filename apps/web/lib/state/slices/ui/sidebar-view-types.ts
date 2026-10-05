import type { ViewFilterClause, ViewFilterOp, ViewFilterValue } from "@/lib/view-model/types";
import type { SidebarTaskRowPresentation } from "./sidebar-task-row-presentation";

export type FilterDimension =
  | "archived"
  | "state"
  | "workflow"
  | "workflowStep"
  | "executorType"
  | "repository"
  | "repositoryGroup"
  | "hasDiff"
  | "hasPR"
  | "isPRReview"
  | "isIssueWatch"
  | "titleMatch";

export type FilterOp = ViewFilterOp;

export type FilterValue = ViewFilterValue;

export type FilterClause = ViewFilterClause<FilterDimension>;

export type SortKey = "state" | "updatedAt" | "lastActivityAt" | "createdAt" | "title" | "custom";
export type SortDirection = "asc" | "desc";
export type SortSpec = { key: SortKey; direction: SortDirection };

export type { SidebarTaskRowPresentation } from "./sidebar-task-row-presentation";

export type GroupKey =
  | "none"
  | "repository"
  | "repositoryGroup"
  | "workflow"
  | "workflowStep"
  | "executorType"
  | "state"
  | "priority";

export type SidebarView = {
  id: string;
  name: string;
  filters: FilterClause[];
  sort: SortSpec;
  group: GroupKey;
  collapsedGroups: string[];
  taskRow?: SidebarTaskRowPresentation;
};

export type SidebarSliceState = {
  syncPending?: boolean;
  serverRevision?: number | null;
  deferredServerState?: SidebarSliceState | null;
  views: SidebarView[];
  activeViewId: string;
  draft: SidebarViewDraft | null;
  /** Last error surfaced by an async backend sync. Consumed by a toast bridge. */
  syncError: string | null;
};

export type SidebarViewDraft = {
  baseViewId: string;
  filters: FilterClause[];
  sort: SortSpec;
  group: GroupKey;
  taskRow?: SidebarTaskRowPresentation;
};
