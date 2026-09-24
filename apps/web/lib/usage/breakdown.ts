import type { UsageGroup, UsageTotals } from "@/lib/api/domains/usage-api";
import { parseStrictRfc3339Timestamp } from "@/lib/utils/strict-timestamp";
import { hitRatio } from "./efficiency";

/** The three Usage panel views; all are client-side roll-ups of the groups. */
export type UsageView = "agent" | "model" | "session";

const UNKNOWN_KEY = "__unknown__";
const DELETED_SESSION_KEY = "__deleted__";

function pickEarlier(first: string | null, second: string | null): string | null {
  if (first === null) return second;
  if (second === null) return first;
  const a = parseStrictRfc3339Timestamp(first);
  const b = parseStrictRfc3339Timestamp(second);
  if (a === null) return second;
  if (b === null) return first;
  return a <= b ? first : second;
}

function pickLater(first: string | null, second: string | null): string | null {
  if (first === null) return second;
  if (second === null) return first;
  const a = parseStrictRfc3339Timestamp(first);
  const b = parseStrictRfc3339Timestamp(second);
  if (a === null) return second;
  if (b === null) return first;
  return a >= b ? first : second;
}

/**
 * Adds two ledger totals. Every field is additive; `output_tokens_complete`
 * ANDs and the timestamps take the min/max. This is how the By agent, By model
 * and By session views are built from the finest-grain groups.
 */
export function addTotals(first: UsageTotals, second: UsageTotals): UsageTotals {
  return {
    scope: first.scope,
    scope_id: first.scope_id,
    tokens_in: first.tokens_in + second.tokens_in,
    tokens_cached_read: first.tokens_cached_read + second.tokens_cached_read,
    tokens_cached_write: first.tokens_cached_write + second.tokens_cached_write,
    tokens_out: first.tokens_out + second.tokens_out,
    tokens_thought: first.tokens_thought + second.tokens_thought,
    tokens_total: first.tokens_total + second.tokens_total,
    cost_subcents: first.cost_subcents + second.cost_subcents,
    event_count: first.event_count + second.event_count,
    estimated_event_count: first.estimated_event_count + second.estimated_event_count,
    unpriced_event_count: first.unpriced_event_count + second.unpriced_event_count,
    output_tokens_complete: first.output_tokens_complete && second.output_tokens_complete,
    first_event_at: pickEarlier(first.first_event_at, second.first_event_at),
    last_event_at: pickLater(first.last_event_at, second.last_event_at),
  };
}

/** The group key for one view. Empty identity falls back to a stable label. */
export function groupKey(group: UsageGroup, view: UsageView): string {
  switch (view) {
    case "agent":
      return group.agent_profile_id || UNKNOWN_KEY;
    case "model":
      return `${group.provider}\u0000${group.model}`;
    case "session":
      return group.session_id ?? DELETED_SESSION_KEY;
  }
}

export type UsageRollupRow = {
  key: string;
  sessionId: string | null;
  agentProfileId: string;
  agentType: string;
  model: string;
  provider: string;
  totals: UsageTotals;
  /** Distinct supporting identities, for the By agent sub-label. */
  agentTypes: string[];
  models: string[];
  providers: string[];
};

function addUnique(list: string[], value: string): void {
  if (value !== "" && !list.includes(value)) list.push(value);
}

/**
 * Rolls the finest-grain groups up to one row per view key. Additive fields
 * are summed and the hit ratio is recomputed from the summed tokens by the
 * caller (never averaged).
 */
export function rollUpGroups(groups: UsageGroup[], view: UsageView): UsageRollupRow[] {
  const byKey = new Map<string, UsageRollupRow>();
  for (const group of groups) {
    const key = groupKey(group, view);
    const existing = byKey.get(key);
    if (existing) {
      existing.totals = addTotals(existing.totals, group.totals);
      addUnique(existing.agentTypes, group.agent_type);
      addUnique(existing.models, group.model);
      addUnique(existing.providers, group.provider);
      continue;
    }
    byKey.set(key, {
      key,
      sessionId: group.session_id,
      agentProfileId: group.agent_profile_id,
      agentType: group.agent_type,
      model: group.model,
      provider: group.provider,
      totals: { ...group.totals },
      agentTypes: group.agent_type === "" ? [] : [group.agent_type],
      models: group.model === "" ? [] : [group.model],
      providers: group.provider === "" ? [] : [group.provider],
    });
  }
  return [...byKey.values()];
}

export type UsageRowStats = {
  hitRatio: number | null;
  costPerPrompt: number | null;
  avgInputPerPrompt: number | null;
  avgOutputPerPrompt: number | null;
};

/**
 * Per-row derived statistics. Every average is null (never NaN) when the row
 * has no prompts. `hitRatio` follows the shared rule: null means the provider
 * did not report caching.
 */
export function usageRowStats(totals: UsageTotals): UsageRowStats {
  const ratio = hitRatio(totals);
  if (totals.event_count <= 0) {
    return { hitRatio: ratio, costPerPrompt: null, avgInputPerPrompt: null, avgOutputPerPrompt: null };
  }
  const inputTokens = totals.tokens_in + totals.tokens_cached_read + totals.tokens_cached_write;
  return {
    hitRatio: ratio,
    costPerPrompt: totals.cost_subcents / totals.event_count,
    avgInputPerPrompt: inputTokens / totals.event_count,
    avgOutputPerPrompt: totals.tokens_out / totals.event_count,
  };
}
