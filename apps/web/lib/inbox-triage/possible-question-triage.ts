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
  updated_at?: string | null;
};

/** Triage lane input: the clarification bundle identity from the Needs-you read. */
export type TriageBundleInput = {
  task_id: string;
  session_id?: string | null;
  task_title: string;
  created_at?: string | null;
};

export type InboxTriageKind = "clarification" | "stale_review" | "possible_question";

export type InboxTriageItem = {
  /** Stable identity used for dedup, ordering, and nudge suppression. */
  id: string;
  kind: InboxTriageKind;
  taskId: string;
  sessionId: string | null;
  title: string;
  updatedAt: string | null;
};

/**
 * A Review task older than this is stale even when it was never interrupted.
 * The value is intentionally generous so ordinary review latency is not noise.
 */
export const STALE_REVIEW_MS = 24 * 60 * 60 * 1000;

// Lower rank wins when a task/session maps to more than one kind: a real
// clarification or permission always outranks a stale review, which outranks
// an advisory possible-question hint.
const KIND_RANK: Record<InboxTriageKind, number> = {
  clarification: 0,
  stale_review: 1,
  possible_question: 2,
};

function dedupeKey(taskId: string, sessionId: string | null): string {
  return `${taskId}:${sessionId ?? ""}`;
}

function itemId(kind: InboxTriageKind, taskId: string, sessionId: string | null): string {
  return `${kind}:${taskId}:${sessionId ?? ""}`;
}

// A possible-question hint additionally carries its producing turn, so the
// nudge identity changes when the same session surfaces a new hint.
function possibleQuestionItemId(
  taskId: string,
  sessionId: string | null,
  turnId: string | null,
): string {
  return `${itemId("possible_question", taskId, sessionId)}:${turnId ?? ""}`;
}

function isStaleReview(task: TriageTaskInput, nowMs: number): boolean {
  if (task.state !== "REVIEW") return false;
  if (task.interrupted) return true;
  if (!task.updated_at) return false;
  const updatedMs = Date.parse(task.updated_at);
  if (Number.isNaN(updatedMs)) return false;
  return nowMs - updatedMs >= STALE_REVIEW_MS;
}

function compareIds(a: string, b: string): number {
  if (a === b) return 0;
  return a < b ? -1 : 1;
}

/**
 * Builds the ordered, deduplicated triage lane. Real clarifications come
 * first, then stale or interrupted Review tasks, then advisory
 * possible-question hints. One task/session appears once, at its highest
 * rank; ties order newest-first with the id as a stable final key.
 */
export function buildInboxTriage(
  tasks: readonly TriageTaskInput[],
  bundles: readonly TriageBundleInput[],
  nowMs: number,
): InboxTriageItem[] {
  const byDedupeKey = new Map<string, InboxTriageItem>();

  const consider = (candidate: InboxTriageItem) => {
    const key = dedupeKey(candidate.taskId, candidate.sessionId);
    const existing = byDedupeKey.get(key);
    if (!existing || KIND_RANK[candidate.kind] < KIND_RANK[existing.kind]) {
      byDedupeKey.set(key, candidate);
    }
  };

  for (const bundle of bundles) {
    const sessionId = bundle.session_id ?? null;
    consider({
      id: itemId("clarification", bundle.task_id, sessionId),
      kind: "clarification",
      taskId: bundle.task_id,
      sessionId,
      title: bundle.task_title,
      updatedAt: bundle.created_at ?? null,
    });
  }

  for (const task of tasks) {
    const sessionId = task.primary_session_id ?? null;
    if (isStaleReview(task, nowMs)) {
      consider({
        id: itemId("stale_review", task.id, sessionId),
        kind: "stale_review",
        taskId: task.id,
        sessionId,
        title: task.title,
        updatedAt: task.updated_at ?? null,
      });
    }
    if (task.possible_question) {
      consider({
        id: possibleQuestionItemId(task.id, sessionId, task.possible_question_turn_id ?? null),
        kind: "possible_question",
        taskId: task.id,
        sessionId,
        title: task.title,
        updatedAt: task.updated_at ?? null,
      });
    }
  }

  return [...byDedupeKey.values()].sort((a, b) => {
    const rank = KIND_RANK[a.kind] - KIND_RANK[b.kind];
    if (rank !== 0) return rank;
    const aMs = a.updatedAt ? Date.parse(a.updatedAt) : Number.NaN;
    const bMs = b.updatedAt ? Date.parse(b.updatedAt) : Number.NaN;
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
  return items.filter((item) => !acknowledged.has(item.id)).map((item) => item.id);
}
