import type { TaskState } from "@/lib/types/http";

/**
 * Triage lane input: the minimal task shape the lane needs. Kept structural so
 * the ordering rules stay independent of any one store projection.
 */
export type TriageTaskInput = {
  id: string;
  title: string;
  state?: TaskState;
  primary_session_id?: string | null;
  possible_question?: boolean | null;
  /**
   * Turn identity behind the hint. It makes a hint surfaced on a new turn a
   * distinct identity, so an acknowledgement of an earlier turn does not
   * suppress the nudge for the new one.
   */
  possible_question_turn_id?: string | null;
  interrupted?: boolean | null;
  /** Legacy marker time; identifies an interruption with no typed episode. */
  interrupted_at?: string | null;
  /** Oldest unresolved session interruption episode. */
  interruption?: { episode_id: string; reason: string; session_id: string } | null;
  /** Active Review episode identity and authoritative entry time. */
  review_episode_id?: string | null;
  review_entered_at?: string | null;
  blocked?: boolean | null;
  updated_at?: string | null;
};

/** Triage lane input: the clarification bundle identity from the Needs-you read. */
export type TriageBundleInput = {
  task_id: string;
  session_id?: string | null;
  task_title: string;
  created_at?: string | null;
};

export type InboxTriageKind =
  | "clarification"
  | "interrupted"
  | "stale_review"
  | "review"
  | "dependency"
  | "possible_question";

/** Why the row exists. Closed so every value has localized copy. */
export type InboxTriageReason =
  | "clarification"
  | "backend_restart"
  | "execution_lost"
  | "interrupted_unknown"
  | "stale_review"
  | "review"
  | "dependency"
  | "possible_question";

export type InboxTriageItem = {
  /** Stable identity used for ordering and nudge suppression. */
  id: string;
  kind: InboxTriageKind;
  reason: InboxTriageReason;
  taskId: string;
  sessionId: string | null;
  title: string;
  /** When the incident began; never the task's last edit for Review rows. */
  since: string | null;
};

/**
 * A Review task whose entry is older than this is stale. The value is
 * intentionally generous so ordinary review latency is not noise.
 */
export const STALE_REVIEW_MS = 24 * 60 * 60 * 1000;

// Lower rank wins when a task maps to more than one kind. An unresolved
// interruption outranks Review: a task that lost its execution is resumable
// work, not a review request. A possible question outranks a fresh review
// because a finished turn already puts its task in Review.
const KIND_RANK: Record<InboxTriageKind, number> = {
  clarification: 0,
  interrupted: 1,
  stale_review: 2,
  possible_question: 3,
  review: 4,
  dependency: 5,
};

/**
 * Kinds that raise the in-app nudge. A fresh review and a dependency wait are
 * standing states listed in the lane, not new incidents to count.
 */
export const NUDGE_KINDS: ReadonlySet<InboxTriageKind> = new Set([
  "clarification",
  "interrupted",
  "stale_review",
  "possible_question",
]);

const INTERRUPTION_REASONS: Record<string, InboxTriageReason> = {
  backend_restart: "backend_restart",
  execution_lost: "execution_lost",
};

function clarificationItem(bundle: TriageBundleInput): InboxTriageItem {
  const sessionId = bundle.session_id ?? null;
  return {
    id: `clarification:${bundle.task_id}:${sessionId ?? ""}`,
    kind: "clarification",
    reason: "clarification",
    taskId: bundle.task_id,
    sessionId,
    title: bundle.task_title,
    since: bundle.created_at ?? null,
  };
}

function interruptedItem(task: TriageTaskInput): InboxTriageItem | null {
  const episode = task.interrupted === false ? null : (task.interruption ?? null);
  if (!episode && !task.interrupted) return null;
  return {
    // The episode keys the identity, so a later interruption is not hidden by
    // an acknowledgement of an earlier one. Unattributed legacy data falls
    // back to the task plus its marker time.
    id: episode
      ? `interrupted:${task.id}:${episode.episode_id}`
      : `interrupted:${task.id}:legacy:${task.interrupted_at ?? ""}`,
    kind: "interrupted",
    reason: (episode && INTERRUPTION_REASONS[episode.reason]) || "interrupted_unknown",
    taskId: task.id,
    sessionId: episode?.session_id ?? task.primary_session_id ?? null,
    title: task.title,
    since: task.interrupted_at ?? null,
  };
}

function reviewItem(task: TriageTaskInput, nowMs: number): InboxTriageItem | null {
  if (task.state !== "REVIEW") return null;
  const enteredMs = task.review_entered_at ? Date.parse(task.review_entered_at) : Number.NaN;
  const stale = !Number.isNaN(enteredMs) && nowMs - enteredMs >= STALE_REVIEW_MS;
  const kind = stale ? "stale_review" : "review";
  return {
    id: `${kind}:${task.id}:${task.review_episode_id ?? ""}`,
    kind,
    reason: kind,
    taskId: task.id,
    sessionId: task.primary_session_id ?? null,
    title: task.title,
    since: task.review_entered_at ?? null,
  };
}

function dependencyItem(task: TriageTaskInput): InboxTriageItem | null {
  if (!task.blocked) return null;
  return {
    id: `dependency:${task.id}`,
    kind: "dependency",
    reason: "dependency",
    taskId: task.id,
    sessionId: task.primary_session_id ?? null,
    title: task.title,
    since: task.updated_at ?? null,
  };
}

function possibleQuestionItem(task: TriageTaskInput): InboxTriageItem | null {
  if (!task.possible_question) return null;
  const sessionId = task.primary_session_id ?? null;
  return {
    // The producing turn is part of the identity, so the nudge changes when
    // the same session surfaces a new hint.
    id: `possible_question:${task.id}:${sessionId ?? ""}:${task.possible_question_turn_id ?? ""}`,
    kind: "possible_question",
    reason: "possible_question",
    taskId: task.id,
    sessionId,
    title: task.title,
    since: task.updated_at ?? null,
  };
}

function compareIds(a: string, b: string): number {
  if (a === b) return 0;
  return a < b ? -1 : 1;
}

/**
 * Builds the ordered triage lane with one row per task, at its highest-ranked
 * kind: real clarifications, then interrupted work, stale reviews, advisory
 * possible-question hints, reviews, and dependency waits. Ties order
 * newest-first with the id as a stable final key.
 */
export function buildInboxTriage(
  tasks: readonly TriageTaskInput[],
  bundles: readonly TriageBundleInput[],
  nowMs: number,
): InboxTriageItem[] {
  const byTask = new Map<string, InboxTriageItem>();

  const consider = (candidate: InboxTriageItem | null) => {
    if (!candidate) return;
    const existing = byTask.get(candidate.taskId);
    if (!existing || KIND_RANK[candidate.kind] < KIND_RANK[existing.kind]) {
      byTask.set(candidate.taskId, candidate);
    }
  };

  for (const bundle of bundles) consider(clarificationItem(bundle));
  for (const task of tasks) {
    consider(interruptedItem(task));
    consider(reviewItem(task, nowMs));
    consider(dependencyItem(task));
    consider(possibleQuestionItem(task));
  }

  return [...byTask.values()].sort((a, b) => {
    const rank = KIND_RANK[a.kind] - KIND_RANK[b.kind];
    if (rank !== 0) return rank;
    const aMs = a.since ? Date.parse(a.since) : Number.NaN;
    const bMs = b.since ? Date.parse(b.since) : Number.NaN;
    const aValid = !Number.isNaN(aMs);
    const bValid = !Number.isNaN(bMs);
    if (aValid && bValid && aMs !== bMs) return bMs - aMs;
    return compareIds(a.id, b.id);
  });
}

/**
 * Returns the identity set of items the operator has not acknowledged yet.
 * Callers persist the acknowledged set; the nudge is derived, never stored as
 * a provider notification.
 */
export function selectUnacknowledgedTriageIds(
  items: readonly InboxTriageItem[],
  acknowledged: ReadonlySet<string>,
): string[] {
  return items
    .filter((item) => NUDGE_KINDS.has(item.kind) && !acknowledged.has(item.id))
    .map((item) => item.id);
}
