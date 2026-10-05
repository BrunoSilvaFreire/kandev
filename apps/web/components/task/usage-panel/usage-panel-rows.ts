import type { UsageGroup, UsageTotals } from "@/lib/api/domains/usage-api";
import { rollUpGroups, type UsageRollupRow, type UsageView } from "@/lib/usage/breakdown";
import {
  cacheStatus,
  epochMillisFromWire,
  estimatedExpiryAt,
  type CacheStatus,
} from "@/lib/usage/efficiency";
import type { PromptUsageEntry } from "@/lib/state/slices/session-runtime/types";

type ProfileLike = { id: string; name?: string };
type SessionLike = { id: string; name?: string };

export type UsageDisplayRow = {
  key: string;
  primary: string;
  secondary: string | null;
  /** Short identity shown only as a hover tooltip, never as the row label. */
  title: string | null;
  totals: UsageTotals;
  isSession: boolean;
  sessionId: string | null;
  cacheStatus: CacheStatus | null;
  expiresAt: number | null;
  lastPrompt: PromptUsageEntry | undefined;
};

export type UsagePanelRowsArgs = {
  view: UsageView;
  groups: UsageGroup[];
  sessions: SessionLike[] | undefined;
  profiles: ProfileLike[];
  deletedProfile: string;
  lastPromptBySession: Record<string, PromptUsageEntry>;
  now: number;
};

function profileName(
  profiles: ProfileLike[],
  agentProfileId: string,
  deletedProfile: string,
  unknown: string,
): string {
  if (agentProfileId === "") return unknown;
  const profile = profiles.find((candidate) => candidate.id === agentProfileId);
  if (profile?.name) return profile.name;
  return deletedProfile;
}

function shortId(id: string | null): string {
  return id ? id.slice(0, 8) : "";
}

/** Short identity for the tooltip when the row label is a fallback, not a name. */
function identityTitle(profiles: ProfileLike[], agentProfileId: string): string | null {
  if (agentProfileId === "") return null;
  const profile = profiles.find((candidate) => candidate.id === agentProfileId);
  return profile?.name ? null : shortId(agentProfileId);
}

/**
 * The session label the task/session tabs use. It prefers the session name,
 * then its agent profile, then a deleted-profile fallback, and never renders a
 * UUID; the short id is kept for the hover tooltip only.
 */
function sessionLabel(
  session: SessionLike | undefined,
  row: UsageRollupRow,
  profiles: ProfileLike[],
  deletedProfile: string,
  unknown: string,
): string {
  if (session?.name) return session.name;
  const profile = profileName(profiles, row.agentProfileId, deletedProfile, "");
  return profile !== "" ? profile : unknown;
}

function joinDetail(parts: string[]): string | null {
  const filtered = parts.filter((part) => part !== "");
  return filtered.length > 0 ? filtered.join(", ") : null;
}

function sessionCache(
  row: UsageRollupRow,
  args: UsagePanelRowsArgs,
): { cacheStatus: CacheStatus; expiresAt: number | null } {
  // Cache warmth is keyed on the newest usage-ledger event, the only observable
  // provider round-trip; messages are ignored.
  const lastUsageEventAt = epochMillisFromWire(row.totals.last_event_at);
  const status = cacheStatus({
    lastUsageEventAt,
    eventCount: row.totals.event_count,
    now: args.now,
  });
  return { cacheStatus: status, expiresAt: estimatedExpiryAt(lastUsageEventAt) };
}

function agentRow(row: UsageRollupRow, args: UsagePanelRowsArgs, unknown: string): UsageDisplayRow {
  return {
    key: row.key,
    primary: profileName(args.profiles, row.agentProfileId, args.deletedProfile, unknown),
    secondary: joinDetail([...row.agentTypes, ...row.models, ...row.providers]),
    title: identityTitle(args.profiles, row.agentProfileId),
    totals: row.totals,
    isSession: false,
    sessionId: null,
    cacheStatus: null,
    expiresAt: null,
    lastPrompt: undefined,
  };
}

function modelRow(row: UsageRollupRow, unknown: string): UsageDisplayRow {
  return {
    key: row.key,
    primary: row.model === "" ? unknown : row.model,
    secondary: row.provider === "" ? null : row.provider,
    title: null,
    totals: row.totals,
    isSession: false,
    sessionId: null,
    cacheStatus: null,
    expiresAt: null,
    lastPrompt: undefined,
  };
}

function sessionRow(
  row: UsageRollupRow,
  args: UsagePanelRowsArgs,
  unknown: string,
): UsageDisplayRow {
  const session = row.sessionId
    ? (args.sessions ?? []).find((candidate) => candidate.id === row.sessionId)
    : undefined;
  const cache = sessionCache(row, args);
  return {
    key: row.key,
    primary: sessionLabel(session, row, args.profiles, args.deletedProfile, unknown),
    secondary: null,
    title: session?.name ? null : shortId(row.sessionId) || null,
    totals: row.totals,
    isSession: true,
    sessionId: row.sessionId,
    cacheStatus: cache.cacheStatus,
    expiresAt: cache.expiresAt,
    lastPrompt: row.sessionId ? args.lastPromptBySession[row.sessionId] : undefined,
  };
}

function emptyTotals(): UsageTotals {
  return {
    scope: "group",
    scope_id: "",
    tokens_in: 0,
    tokens_cached_read: 0,
    tokens_cached_write: 0,
    tokens_out: 0,
    tokens_thought: 0,
    tokens_total: 0,
    cost_subcents: 0,
    event_count: 0,
    estimated_event_count: 0,
    unpriced_event_count: 0,
    output_tokens_complete: true,
    first_event_at: null,
    last_event_at: null,
  };
}

/** Sessions with no ledger rows are added so the panel never hides a live agent. */
function emptySessionRow(
  session: SessionLike,
  args: UsagePanelRowsArgs,
  unknown: string,
): UsageDisplayRow {
  const totals = emptyTotals();
  const row: UsageRollupRow = {
    key: session.id,
    sessionId: session.id,
    agentProfileId: "",
    agentType: "",
    model: "",
    provider: "",
    totals,
    agentTypes: [],
    models: [],
    providers: [],
  };
  const cache = sessionCache(row, args);
  return {
    key: `session:${session.id}`,
    primary: sessionLabel(session, row, args.profiles, args.deletedProfile, unknown),
    secondary: null,
    title: session.name ? null : shortId(session.id) || null,
    totals,
    isSession: true,
    sessionId: session.id,
    cacheStatus: cache.cacheStatus,
    expiresAt: cache.expiresAt,
    lastPrompt: args.lastPromptBySession[session.id],
  };
}

/**
 * Builds the display rows for the active view. Agent and model rows are pure
 * roll-ups; the session view additionally merges the task's store sessions so
 * a session with no ledger rows still appears ("No usage reported").
 */
export function buildUsageDisplayRows(
  args: UsagePanelRowsArgs,
  unknown: string,
): UsageDisplayRow[] {
  const rolled = rollUpGroups(args.groups, args.view);
  const rows = rolled.map((row) => {
    if (args.view === "agent") return agentRow(row, args, unknown);
    if (args.view === "model") return modelRow(row, unknown);
    return sessionRow(row, args, unknown);
  });
  if (args.view === "session") {
    const seen = new Set(rows.map((row) => row.sessionId));
    for (const session of args.sessions ?? []) {
      if (!seen.has(session.id)) rows.push(emptySessionRow(session, args, unknown));
    }
  }
  return rows;
}
