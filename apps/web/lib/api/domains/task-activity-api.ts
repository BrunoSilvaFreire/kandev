import { getWebSocketClient } from "@/lib/ws/connection";

// i18n-exempt: diagnostic thrown to the caller, never rendered.
const WS_CLIENT_UNAVAILABLE = "WebSocket client not available";

export type TaskStepTransition = {
  id: number;
  session_id?: string;
  from_step_id?: string;
  to_step_id?: string;
  trigger: string;
  trigger_detail?: string;
  actor_kind: string;
  occurred_at: string;
};

export type TaskActivitySession = {
  id: string;
  name?: string;
  agent_profile_id?: string;
  state: string;
  started_at: string;
  completed_at?: string;
};

export type TaskActivityRoute = {
  id: string;
  destination_step_id: string;
  source_session_id?: string;
  destination_session_id?: string;
  agent_profile_id?: string;
  start_policy?: string;
  end_policy?: string;
  outcome: string;
  reason: string;
  /** Optional closed-key JSON with the decision's extra inputs. */
  decision_detail?: string;
  created_at: string;
};

export type TaskActivityDocumentRevision = {
  id: string;
  document_key: string;
  revision_number: number;
  title: string;
  author_kind: string;
  author_name: string;
  source_session_id?: string;
  source_workflow_step_id?: string;
  created_at: string;
};

export type TaskActivityReviewRun = {
  id: string;
  session_id?: string;
  status: string;
  trigger?: string;
  summary?: string;
  finding_count: number;
  created_at: string;
  completed_at?: string;
};

export type TaskActivityEvent = {
  kind:
    | "transition"
    | "session_created"
    | "session_completed"
    | "route"
    | "document_revision"
    | "review_run";
  id: string;
  occurred_at: string;
  transition?: TaskStepTransition;
  session?: TaskActivitySession;
  route?: TaskActivityRoute;
  document_revision?: TaskActivityDocumentRevision;
  review_run?: TaskActivityReviewRun;
};

export type TaskActivityPage = {
  events: TaskActivityEvent[];
  has_more: boolean;
  next_cursor?: string;
};

export type StepTransitionCount = { from_step_id: string; to_step_id: string; count: number };

export type StepVisitSession = {
  session_id: string;
  agent_profile_id?: string;
  workflow_step_transition_id?: number;
  occurred_at: string;
};

export type StepVisit = {
  step_id: string;
  count: number;
  sessions: StepVisitSession[];
};

export type TaskTransitionSummary = {
  counts: StepTransitionCount[];
  visits: StepVisit[];
};

const DEFAULT_LIMIT = 50;

/** List one page of the merged task activity stream (optionally one step). */
export async function listTaskActivity(
  taskId: string,
  options: { stepId?: string; limit?: number; cursor?: string } = {},
): Promise<TaskActivityPage> {
  const client = getWebSocketClient();
  if (!client) throw new Error(WS_CLIENT_UNAVAILABLE);
  const payload: Record<string, string | number> = {
    task_id: taskId,
    limit: options.limit ?? DEFAULT_LIMIT,
  };
  if (options.stepId) payload.step_id = options.stepId;
  if (options.cursor) payload.cursor = options.cursor;
  const response = await client.request("task.activity.list", payload);
  const page = response as TaskActivityPage;
  return {
    events: page?.events ?? [],
    has_more: Boolean(page?.has_more),
    next_cursor: page?.next_cursor,
  };
}

/** Load committed directed-pair counts and per-step visit counts for the header. */
export async function getTaskTransitionSummary(taskId: string): Promise<TaskTransitionSummary> {
  const client = getWebSocketClient();
  if (!client) throw new Error(WS_CLIENT_UNAVAILABLE);
  const response = await client.request("task.transition.counts", { task_id: taskId });
  const summary = response as TaskTransitionSummary;
  return { counts: summary?.counts ?? [], visits: summary?.visits ?? [] };
}
