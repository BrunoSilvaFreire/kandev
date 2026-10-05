import { getSessionStorage, setSessionStorage } from "@/lib/local-storage";

// Session panels the user explicitly closed in a task environment. Absence
// from the serialized layout already hides the panel; this set is what stops
// session reconciliation from re-opening it while the session still exists.
const DOCKVIEW_ENV_CLOSED_SESSIONS_PREFIX = "kandev.dockview.env-closed-sessions-v1.";

/** Session ids the user closed in this env. */
export function getEnvClosedSessionIds(envId: string | null): string[] {
  if (!envId) return [];
  const value = getSessionStorage<string[]>(`${DOCKVIEW_ENV_CLOSED_SESSIONS_PREFIX}${envId}`, []);
  return Array.isArray(value) ? value.filter((id) => typeof id === "string" && id.length > 0) : [];
}

/** Persist the closed-session set for this env. */
export function setEnvClosedSessionIds(envId: string | null, ids: string[]): void {
  if (!envId) return;
  setSessionStorage(`${DOCKVIEW_ENV_CLOSED_SESSIONS_PREFIX}${envId}`, [...new Set(ids)]);
}

/** Mark one session panel closed in this env. */
export function markEnvSessionClosed(envId: string | null, sessionId: string): void {
  if (!envId || !sessionId) return;
  setEnvClosedSessionIds(envId, [...getEnvClosedSessionIds(envId), sessionId]);
}

/** Clear the closed mark when a session tab is opened again. */
export function clearEnvSessionClosed(envId: string | null, sessionId: string): void {
  if (!envId || !sessionId) return;
  setEnvClosedSessionIds(
    envId,
    getEnvClosedSessionIds(envId).filter((id) => id !== sessionId),
  );
}
