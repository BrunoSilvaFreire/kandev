import type { UsageBreakdownQuery, UsageBreakdownRow } from "@/lib/types/provider-usage";

/** A filter a user can add and remove from the Breakdown tab. */
export type BreakdownFilterKey = "provider" | "model" | "agentType" | "taskId" | "sessionId" | "q";

export type BreakdownFilterChip = {
  key: BreakdownFilterKey;
  value: string;
};

const FILTER_LABEL_FIELDS: Record<BreakdownFilterKey, "taskLabel" | "sessionLabel" | null> = {
  provider: null,
  model: null,
  agentType: null,
  taskId: "taskLabel",
  sessionId: "sessionLabel",
  q: null,
};

/**
 * Advance a row selection to the next dimension. Provider narrows to model,
 * model to session, and every non-session dimension to the sessions that make
 * up the row. The day bucket has no matching filter, so it only switches to
 * the session view.
 */
export function drillDown(query: UsageBreakdownQuery, row: UsageBreakdownRow): UsageBreakdownQuery {
  const base = { ...query, offset: 0 };
  switch (query.groupBy ?? "session") {
    case "provider":
      return { ...base, groupBy: "model", provider: row.key };
    case "model":
      return { ...base, groupBy: "session", model: row.key };
    case "agent":
      return { ...base, groupBy: "session", agentType: row.key };
    case "task":
      return { ...base, groupBy: "session", taskId: row.key, taskLabel: row.label };
    case "day":
      return { ...base, groupBy: "session" };
    default:
      return { ...base, groupBy: "model", sessionId: row.key, sessionLabel: row.label };
  }
}

/** The active filters as removable chips, id filters carrying their row label. */
export function activeFilterChips(query: UsageBreakdownQuery): BreakdownFilterChip[] {
  const chips: BreakdownFilterChip[] = [];
  const push = (key: BreakdownFilterKey, value: string | undefined) => {
    if (!value) return;
    const labelField = FILTER_LABEL_FIELDS[key];
    const label = labelField ? query[labelField] : undefined;
    chips.push({ key, value: label ?? value });
  };
  push("provider", query.provider);
  push("model", query.model);
  push("agentType", query.agentType);
  push("taskId", query.taskId);
  push("sessionId", query.sessionId);
  push("q", query.q);
  return chips;
}

/**
 * Remove one filter, keeping the current view and paging back to the start.
 * Removed fields are set to undefined rather than deleted so a caller that
 * merges the result over the previous query actually clears them.
 */
export function removeFilter(
  query: UsageBreakdownQuery,
  key: BreakdownFilterKey,
): UsageBreakdownQuery {
  const next: UsageBreakdownQuery = { ...query, offset: 0 };
  if (key === "provider") next.provider = undefined;
  if (key === "model") next.model = undefined;
  if (key === "agentType") next.agentType = undefined;
  if (key === "taskId") {
    next.taskId = undefined;
    next.taskLabel = undefined;
  }
  if (key === "sessionId") {
    next.sessionId = undefined;
    next.sessionLabel = undefined;
  }
  if (key === "q") next.q = undefined;
  return next;
}

/** Drop every filter at once. */
export function clearFilters(query: UsageBreakdownQuery): UsageBreakdownQuery {
  return removeFilter(
    removeFilter(
      removeFilter(
        removeFilter(removeFilter(removeFilter(query, "provider"), "model"), "agentType"),
        "taskId",
      ),
      "sessionId",
    ),
    "q",
  );
}

/** Share of a row's tokens against the filtered total, clamped to [0, 100]. */
export function sharePct(tokens: number, total: number): number {
  if (total <= 0) return 0;
  return Math.min(100, Math.max(0, (tokens / total) * 100));
}
