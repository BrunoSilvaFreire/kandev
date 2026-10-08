// Shared, process-local acknowledgement of surfaced triage identities.
//
// The inbox navigation badge and the inbox page are separate component trees,
// so component-local state would let the badge keep showing a nudge the page
// already dismissed. Persistence is best-effort browser storage; the store is
// the single source of truth for both consumers in the current tab.

const STORAGE_KEY = "kandev.inbox.triage.acknowledged";

// Stable empty snapshot for server rendering/hydration; the client re-renders
// from storage immediately after subscribing.
const EMPTY_ACKNOWLEDGED: ReadonlySet<string> = new Set();

let acknowledged: ReadonlySet<string> | null = null;
const listeners = new Set<() => void>();

function storage(): Storage | null {
  return typeof window === "undefined" ? null : window.localStorage;
}

function parse(raw: string | null): Set<string> {
  if (!raw) return new Set();
  try {
    const parsed: unknown = JSON.parse(raw);
    if (!Array.isArray(parsed)) return new Set();
    return new Set(parsed.filter((value): value is string => typeof value === "string"));
  } catch {
    return new Set();
  }
}

function read(): ReadonlySet<string> {
  if (acknowledged === null) {
    acknowledged = parse(storage()?.getItem(STORAGE_KEY) ?? null);
  }
  return acknowledged;
}

function sameSet(left: ReadonlySet<string>, right: ReadonlySet<string>): boolean {
  if (left.size !== right.size) return false;
  for (const value of left) if (!right.has(value)) return false;
  return true;
}

function commit(next: Set<string>): void {
  acknowledged = next;
  try {
    storage()?.setItem(STORAGE_KEY, JSON.stringify([...next]));
  } catch {
    // Non-fatal: a blocked storage backend only means the nudge is not deduped.
  }
  for (const listener of listeners) listener();
}

export function subscribeAcknowledgedTriage(listener: () => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

export function getAcknowledgedTriageIds(): ReadonlySet<string> {
  return read();
}

export function getServerAcknowledgedTriageIds(): ReadonlySet<string> {
  return EMPTY_ACKNOWLEDGED;
}

/** Replaces the acknowledged set with the supplied identities. */
export function acknowledgeTriageItems(ids: Iterable<string>): void {
  const next = new Set(ids);
  if (sameSet(next, read())) return;
  commit(next);
}

/** Drops acknowledgements whose identity is no longer surfaced. */
export function pruneAcknowledgedTriage(present: ReadonlySet<string>): void {
  const current = read();
  const next = new Set<string>();
  for (const id of current) {
    if (present.has(id)) next.add(id);
  }
  if (next.size === current.size) return;
  commit(next);
}

/** Test-only: clear the in-memory cache so the next read re-reads storage. */
export function resetAcknowledgedTriageForTests(): void {
  acknowledged = null;
}
