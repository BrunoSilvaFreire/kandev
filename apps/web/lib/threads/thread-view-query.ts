import type { KanbanState, WorkflowSnapshotData } from "@/lib/state/slices/kanban/types";
import type { ForegroundActivity, TaskPriority, TaskState } from "@/lib/types/http";
import { taskPRInfoFromSummary, type TaskPRInfo } from "@/lib/task-pr-info";
import {
  applyViewFilters,
  applyViewGroup,
  repositoryGroupKeyAndLabel,
  threadCandidateDimensionValue,
  type RepositoryGroup,
  type ViewGroup,
} from "@/lib/view-model";
import { t } from "@/lib/i18n";
import { selectActiveThreads, type ActiveThread } from "./active-threads";
import type {
  ThreadGroupKey,
  ThreadSortKey,
  ThreadView,
  ThreadViewDraft,
} from "@/lib/state/slices/ui/thread-view-types";

type KanbanTask = KanbanState["tasks"][number];

export type ThreadStatus = "needs_action" | "running" | "waiting" | "ready_for_review";
export type ThreadTaskType = "standard" | "pull_request_review" | "issue_watch";

/** Bounded task data used by the Threads query and header controls. */
export type ThreadCandidate = ActiveThread & {
  workflowStepId: string;
  taskState: TaskState | null;
  foregroundActivity: ForegroundActivity | null;
  interrupted: boolean;
  isOnLastWorkflowStep: boolean;
  priority: TaskPriority | null;
  blocked: boolean;
  labels: string[];
  taskOrigin: string;
  taskType: ThreadTaskType;
  repositoryIds: string[];
  primaryAgentProfileId: string | null;
  primaryAgentName: string | null;
  executorType: string | null;
  sessionCount: number;
  threadStatus: ThreadStatus;
  hasDiff: boolean;
  hasPR: boolean;
  prInfo?: TaskPRInfo;
  prNeedsAttention: boolean;
  hasActiveError: boolean;
  hasMultipleSessions: boolean;
  updatedAt: string | null;
  createdAt: string | null;
};

export type ThreadViewQueryOptions = {
  workspaceId?: string | null;
  requestedTaskId?: string | null;
  draft?: ThreadViewDraft | null;
  /** Transient removal intent applies before every admission path, including deep links. */
  excludedTaskIds?: ReadonlySet<string>;
  /** Ordered Repository Groups backing the `repositoryGroup` filter/group. */
  repositoryGroups?: readonly RepositoryGroup[];
  /** Display names for repository ids used in group headings. */
  repositoryNames?: ReadonlyMap<string, string>;
};

export type ThreadViewQueryResult = {
  candidates: ThreadCandidate[];
  /** Candidates that satisfy the saved scope and every active filter. */
  matchingCandidates: ThreadCandidate[];
  /** Complete current set used to retain admitted columns during live updates. */
  stableCandidates: ThreadCandidate[];
  admittedCandidates: ThreadCandidate[];
  /** Admitted candidates grouped by the view's active group key. */
  groupedCandidates: ViewGroup<ThreadCandidate>[];
  matchingCount: number;
  /** Number of temporary deep-link candidates outside the saved query. */
  temporaryAdmissionCount: number;
  hiddenCount: number;
  effectiveView: ThreadView;
  fingerprint: string;
};

function taskOrigin(task: KanbanTask): string {
  if (task.origin) return task.origin;
  const metadataOrigin = task.metadata?.origin;
  return typeof metadataOrigin === "string" && metadataOrigin.length > 0
    ? metadataOrigin
    : "unknown";
}

function taskType(task: KanbanTask): ThreadTaskType {
  if (task.isPRReview) return "pull_request_review";
  if (task.isIssueWatch) return "issue_watch";
  return "standard";
}

function hasGitChange(task: KanbanTask): boolean {
  const git = task.statusSummary?.git;
  return Boolean(
    git && ((git.changed_files ?? 0) > 0 || (git.additions ?? 0) > 0 || (git.deletions ?? 0) > 0),
  );
}

function hasPullRequest(task: KanbanTask): boolean {
  return (task.statusSummary?.pull_request?.count ?? 0) > 0;
}

function resolveThreadStatus(thread: ActiveThread): ThreadStatus {
  if (thread.pendingAction || thread.taskPendingAction) return "needs_action";
  if (thread.taskState === "REVIEW" || thread.reviewStatus === "pending") {
    return "ready_for_review";
  }
  if (thread.sessionState === "RUNNING" || thread.sessionState === "STARTING") {
    return "running";
  }
  return "waiting";
}

// eslint-disable-next-line complexity -- Projects the complete bounded candidate contract from two authoritative task sources.
function projectCandidate(
  task: KanbanTask,
  thread: ActiveThread,
  isOnLastWorkflowStep: boolean,
): ThreadCandidate {
  const summary = task.statusSummary;
  const sessionCount = Math.max(1, task.sessionCount ?? 1);
  return {
    ...thread,
    workflowStepId: task.workflowStepId,
    taskState: task.state ?? null,
    foregroundActivity: summary?.foreground_activity ?? task.foregroundActivity ?? null,
    interrupted: task.interrupted === true,
    isOnLastWorkflowStep,
    priority: task.priority ?? null,
    blocked: task.blocked ?? false,
    labels: [...(task.labels ?? [])],
    taskOrigin: taskOrigin(task),
    taskType: taskType(task),
    repositoryIds: (task.repositories ?? [])
      .map((repository) => repository.repository_id)
      .filter((repositoryId): repositoryId is string => repositoryId.length > 0),
    primaryAgentProfileId: task.primaryAgentProfileId ?? null,
    primaryAgentName: task.primaryAgentName ?? null,
    executorType: task.primaryExecutorType ?? null,
    sessionCount,
    threadStatus: resolveThreadStatus(thread),
    hasDiff: hasGitChange(task),
    hasPR: hasPullRequest(task),
    prInfo: taskPRInfoFromSummary(summary),
    prNeedsAttention: task.statusSummary?.pull_request?.attention === true,
    hasActiveError: task.statusSummary?.active_error != null,
    hasMultipleSessions: sessionCount > 1,
    updatedAt: task.updatedAt ?? summary?.updated_at ?? null,
    createdAt: task.createdAt ?? null,
  };
}

function taskMapForSnapshots(
  snapshots: Record<string, WorkflowSnapshotData>,
  workspaceId?: string | null,
): Map<string, KanbanTask> {
  const tasks = new Map<string, KanbanTask>();
  for (const snapshot of Object.values(snapshots)) {
    for (const task of snapshot.tasks) {
      if (workspaceId && task.workspaceId && task.workspaceId !== workspaceId) continue;
      tasks.set(task.id, task);
    }
  }
  return tasks;
}

function lastWorkflowStepId(snapshot: WorkflowSnapshotData): string | null {
  let lastStep: WorkflowSnapshotData["steps"][number] | undefined;
  for (const step of snapshot.steps) {
    if (!lastStep || step.position > lastStep.position) lastStep = step;
  }
  return lastStep?.id ?? null;
}

/** Project the current workspace's eligible tasks without applying a workflow filter. */
export function selectThreadCandidates(
  snapshots: Record<string, WorkflowSnapshotData>,
  options: Pick<ThreadViewQueryOptions, "workspaceId"> = {},
): ThreadCandidate[] {
  const tasks = taskMapForSnapshots(snapshots, options.workspaceId);
  const lastStepIdByWorkflowId = new Map(
    Object.values(snapshots).map((snapshot) => [snapshot.workflowId, lastWorkflowStepId(snapshot)]),
  );
  return selectActiveThreads(snapshots)
    .map((thread) => {
      const task = tasks.get(thread.taskId);
      return task
        ? projectCandidate(
            task,
            thread,
            lastStepIdByWorkflowId.get(thread.workflowId) === task.workflowStepId,
          )
        : null;
    })
    .filter((candidate): candidate is ThreadCandidate => candidate !== null);
}

type ThreadGroupContext = {
  repositoryGroups?: readonly RepositoryGroup[];
  repositoryNames?: ReadonlyMap<string, string>;
};

function threadGroupExtractor(
  groupKey: ThreadGroupKey,
  context: ThreadGroupContext,
): (candidate: ThreadCandidate) => { key: string; label: string } {
  const ungrouped = () => t("sidebar:groupUnassigned");
  switch (groupKey) {
    case "repository":
      return (candidate) => {
        const first = candidate.repositoryIds[0];
        if (!first) return { key: "__unassigned__", label: ungrouped() };
        return { key: first, label: context.repositoryNames?.get(first) ?? first };
      };
    case "repositoryGroup":
      return (candidate) =>
        repositoryGroupKeyAndLabel(
          candidate.repositoryIds,
          context.repositoryGroups ?? [],
          ungrouped(),
        );
    case "workflow":
      return (candidate) => ({
        key: candidate.workflowId || "__unassigned__",
        label: candidate.workflowName || candidate.workflowId || ungrouped(),
      });
    case "state":
      return (candidate) => ({
        key: candidate.taskState ?? "__unassigned__",
        label: candidate.taskState ?? ungrouped(),
      });
    case "priority":
      return (candidate) => ({
        key: candidate.priority ?? "__unassigned__",
        label: candidate.priority ?? ungrouped(),
      });
    case "none":
    default:
      return () => ({ key: "__all__", label: "" });
  }
}

const PRIORITY_RANK: Record<string, number> = {
  critical: 0,
  high: 1,
  medium: 2,
  low: 3,
};

const TASK_STATE_RANK: Record<string, number> = {
  CREATED: 0,
  SCHEDULING: 1,
  TODO: 2,
  IN_PROGRESS: 3,
  WAITING_FOR_INPUT: 4,
  BLOCKED: 5,
  REVIEW: 6,
  COMPLETED: 7,
  FAILED: 8,
  CANCELLED: 9,
};

function compareStrings(left: string | null, right: string | null): number {
  if (left === right) return 0;
  if (left === null) return 1;
  if (right === null) return -1;
  return left.localeCompare(right);
}

function compareNumbers(left: number, right: number): number {
  if (left === right) return 0;
  return left < right ? -1 : 1;
}

function dateRank(value: string | null): number | null {
  if (!value) return null;
  const parsed = Date.parse(value);
  return Number.isNaN(parsed) ? null : parsed;
}

function compareDates(left: string | null, right: string | null): number {
  const leftRank = dateRank(left);
  const rightRank = dateRank(right);
  if (leftRank === null || rightRank === null) return compareStrings(left, right);
  return compareNumbers(leftRank, rightRank);
}

function compareRecentDates(left: string | null, right: string | null): number {
  const leftRank = dateRank(left);
  const rightRank = dateRank(right);
  if (leftRank === null) return rightRank === null ? compareStrings(left, right) : 1;
  if (rightRank === null) return -1;
  return compareNumbers(rightRank, leftRank);
}

function attentionRank(status: ThreadStatus): number {
  switch (status) {
    case "needs_action":
      return 0;
    case "running":
    case "ready_for_review":
      return 1;
    case "waiting":
      return 2;
  }
}

// eslint-disable-next-line complexity -- Each supported sort key has an explicit deterministic comparison.
function compareSortKey(left: ThreadCandidate, right: ThreadCandidate, key: ThreadSortKey): number {
  switch (key) {
    case "attention": {
      const bucket = compareNumbers(
        attentionRank(left.threadStatus),
        attentionRank(right.threadStatus),
      );
      return bucket !== 0 ? bucket : compareRecentDates(left.lastActivityAt, right.lastActivityAt);
    }
    case "lastActivityAt":
      return compareDates(left.lastActivityAt, right.lastActivityAt);
    case "updatedAt":
      return compareDates(left.updatedAt, right.updatedAt);
    case "createdAt":
      return compareDates(left.createdAt, right.createdAt);
    case "title":
      return left.title.localeCompare(right.title);
    case "taskState":
      return compareNumbers(
        TASK_STATE_RANK[left.taskState ?? ""] ?? Number.MAX_SAFE_INTEGER,
        TASK_STATE_RANK[right.taskState ?? ""] ?? Number.MAX_SAFE_INTEGER,
      );
    case "workflow": {
      const workflow = left.workflowName.localeCompare(right.workflowName);
      return workflow !== 0
        ? workflow
        : (left.stepTitle?.localeCompare(right.stepTitle ?? "") ?? 0);
    }
    case "priority":
      return compareNumbers(
        PRIORITY_RANK[left.priority ?? ""] ?? Number.MAX_SAFE_INTEGER,
        PRIORITY_RANK[right.priority ?? ""] ?? Number.MAX_SAFE_INTEGER,
      );
    case "primaryAgent":
      return compareStrings(
        left.primaryAgentName ?? left.primaryAgentProfileId,
        right.primaryAgentName ?? right.primaryAgentProfileId,
      );
  }
}

function compareCandidates(
  left: ThreadCandidate,
  right: ThreadCandidate,
  view: ThreadView,
): number {
  const direction = view.sort.direction === "desc" ? -1 : 1;
  const sorted = compareSortKey(left, right, view.sort.key);
  if (sorted !== 0) return sorted * direction;
  return left.taskId.localeCompare(right.taskId);
}

function cloneViewWithDraft(
  view: ThreadView,
  draft: ThreadViewDraft | null | undefined,
): ThreadView {
  if (!draft || draft.baseViewId !== view.id) return view;
  return {
    ...view,
    taskScope:
      draft.taskScope.mode === "all"
        ? { mode: "all", taskIds: [] }
        : { mode: "selected", taskIds: [...draft.taskScope.taskIds] },
    filters: draft.filters.map((filter) => ({
      ...filter,
      value: Array.isArray(filter.value) ? [...filter.value] : filter.value,
    })),
    sort: { ...draft.sort },
    group: draft.group,
    maxColumns: draft.maxColumns,
    layout: draft.layout,
    autoHideComposer: draft.autoHideComposer,
  };
}

function queryFingerprint(view: ThreadView): string {
  return JSON.stringify({
    id: view.id,
    taskScope: view.taskScope,
    filters: view.filters,
    sort: view.sort,
    maxColumns: view.maxColumns,
  });
}

function applyTaskScope(
  candidates: ThreadCandidate[],
  scope: ThreadView["taskScope"],
): ThreadCandidate[] {
  if (scope.mode === "all") return candidates;
  const selected = new Set(scope.taskIds);
  return candidates.filter((candidate) => selected.has(candidate.taskId));
}

function admitCandidates(
  matching: ThreadCandidate[],
  deepLinked: ThreadCandidate | null,
  maxColumns: number | null,
): ThreadCandidate[] {
  const ordinary = maxColumns === null ? [...matching] : matching.slice(0, maxColumns);
  if (!deepLinked || ordinary.some((candidate) => candidate.taskId === deepLinked.taskId)) {
    return ordinary;
  }
  if (maxColumns === null || ordinary.length < maxColumns) return [...ordinary, deepLinked];
  return [...ordinary.slice(0, -1), deepLinked];
}

/** Apply the effective Threads view and return only the task columns to mount. */
export function queryThreadView(
  snapshots: Record<string, WorkflowSnapshotData>,
  view: ThreadView,
  options: ThreadViewQueryOptions = {},
): ThreadViewQueryResult {
  const effectiveView = cloneViewWithDraft(view, options.draft);
  const candidates = selectThreadCandidates(snapshots, options).filter(
    (candidate) => !options.excludedTaskIds?.has(candidate.taskId),
  );
  const scoped = applyTaskScope(candidates, effectiveView.taskScope);
  const matchingCandidates = applyViewFilters(
    scoped,
    effectiveView.filters,
    (candidate, dimension) => threadCandidateDimensionValue(candidate, dimension, options),
  ).sort((left, right) => compareCandidates(left, right, effectiveView));
  const deepLinkCandidate = options.requestedTaskId
    ? (candidates.find((candidate) => candidate.taskId === options.requestedTaskId) ?? null)
    : null;
  const admittedCandidates = admitCandidates(
    matchingCandidates,
    deepLinkCandidate,
    effectiveView.maxColumns,
  );
  const matchingIds = new Set(matchingCandidates.map((candidate) => candidate.taskId));
  const isTemporaryAdmission =
    deepLinkCandidate !== null && !matchingIds.has(deepLinkCandidate.taskId);
  const stableCandidates = isTemporaryAdmission
    ? [...matchingCandidates, deepLinkCandidate]
    : matchingCandidates;
  const admittedIds = new Set(admittedCandidates.map((candidate) => candidate.taskId));
  const hiddenCount = matchingCandidates.filter(
    (candidate) => !admittedIds.has(candidate.taskId),
  ).length;
  const { groups } = applyViewGroup(
    admittedCandidates,
    effectiveView.group,
    threadGroupExtractor(effectiveView.group, options),
  );
  return {
    candidates,
    matchingCandidates,
    stableCandidates,
    admittedCandidates,
    groupedCandidates: groups,
    matchingCount: matchingCandidates.length,
    temporaryAdmissionCount: isTemporaryAdmission ? 1 : 0,
    hiddenCount,
    effectiveView,
    fingerprint: queryFingerprint(effectiveView),
  };
}
