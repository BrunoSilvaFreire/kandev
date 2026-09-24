import { fetchJson, type ApiRequestOptions } from "../client";

/**
 * Mirrors `TaskUsageTotalsDTO` (`apps/backend/internal/task/dto/usage_totals.go`).
 * Every field is always present; `first_event_at` / `last_event_at` serialize
 * to JSON null when the scope has no contributing usage rows.
 */
export type UsageTotals = {
  scope: "task" | "session" | "group";
  scope_id: string;
  tokens_in: number;
  tokens_cached_read: number;
  tokens_cached_write: number;
  tokens_out: number;
  tokens_thought: number;
  tokens_total: number;
  cost_subcents: number;
  event_count: number;
  estimated_event_count: number;
  unpriced_event_count: number;
  output_tokens_complete: boolean;
  first_event_at: string | null;
  last_event_at: string | null;
};

export function getTaskUsageTotals(
  taskId: string,
  options?: ApiRequestOptions,
): Promise<UsageTotals> {
  return fetchJson<UsageTotals>(`/api/v1/tasks/${taskId}/usage`, options);
}

export function getSessionUsageTotals(
  taskId: string,
  sessionId: string,
  options?: ApiRequestOptions,
): Promise<UsageTotals> {
  return fetchJson<UsageTotals>(`/api/v1/tasks/${taskId}/sessions/${sessionId}/usage`, options);
}

/**
 * One finest-grain group of the task ledger: every row sharing the same
 * (session, agent profile, agent type, model, provider). `session_id` is null
 * for rows whose session was deleted.
 */
export type UsageGroup = {
  session_id: string | null;
  agent_profile_id: string;
  agent_type: string;
  model: string;
  provider: string;
  totals: UsageTotals;
};

/** Mirrors `TaskUsageBreakdownDTO`. */
export type UsageBreakdown = {
  task_id: string;
  task: UsageTotals;
  groups: UsageGroup[];
};

export function getTaskUsageBreakdown(
  taskId: string,
  options?: ApiRequestOptions,
): Promise<UsageBreakdown> {
  return fetchJson<UsageBreakdown>(`/api/v1/tasks/${taskId}/usage/breakdown`, options);
}
