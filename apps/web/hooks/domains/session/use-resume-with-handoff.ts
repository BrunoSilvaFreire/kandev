import { useCallback, useEffect, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { resumeWithHandoff } from "@/lib/api/domains/session-api";
import { ApiError } from "@/lib/api/client";
import { CACHE_EXPIRY_MS } from "@/lib/usage/efficiency";
import type { TaskSessionState } from "@/lib/types/http";
import { t } from "@/lib/i18n";

// Shared with the session efficiency inspector so both use one definition.
export { CACHE_EXPIRY_MS };

export const RESUME_HANDOFF_AGENT_ID = "builtin-extract-resume-handoff";

export type ResumeHandoffCandidateInput = {
  state: TaskSessionState | undefined;
  isAgentBusy: boolean;
  hasLiveExecution: boolean;
  hasAgentMessage: boolean;
  lastUsageEventAt: number | null;
  now: number;
};

/**
 * Pure eligibility check for the resume-with-handoff offer. True only for a
 * settled, idle session with a live agent, an agent message, and a newest
 * usage-ledger event older than the cache-expiry window.
 */
export function isResumeHandoffCandidate(input: ResumeHandoffCandidateInput): boolean {
  if (input.state !== "WAITING_FOR_INPUT") return false;
  if (input.isAgentBusy) return false;
  if (!input.hasLiveExecution) return false;
  if (!input.hasAgentMessage) return false;
  if (input.lastUsageEventAt === null) return false;
  return input.now - input.lastUsageEventAt >= CACHE_EXPIRY_MS;
}

export type ResumeWithHandoffOptions = {
  /**
   * Called with the composed prompt when the reset succeeded but the send
   * failed, so the caller can place it in the composer instead of losing it.
   */
  onHandoffNotSent?: (composedPrompt: string) => void;
};

export type ResumeWithHandoffResult = {
  run: (instructions?: string) => Promise<boolean>;
  isRunning: boolean;
  error: string | null;
  clearError: () => void;
};

/**
 * Delegates to the backend's ResumeWithHandoff endpoint, which extracts a
 * facts-only handoff from the transcript, resets the live agent's context, and
 * dispatches the handoff and draft instructions as the first prompt.
 */
export function useResumeWithHandoff(
  taskId: string | null,
  sessionId: string | null,
  options: ResumeWithHandoffOptions = {},
): ResumeWithHandoffResult {
  const [isRunning, setIsRunning] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const clearContextWindow = useAppStore((state) => state.clearContextWindow);
  const { onHandoffNotSent } = options;

  // The composer is not remounted when the active session changes, so a
  // previous session's failure must not surface on the next one.
  useEffect(() => {
    setError(null);
  }, [sessionId]);

  const clearError = useCallback(() => setError(null), []);

  const run = useCallback(
    async (instructions?: string): Promise<boolean> => {
      if (!taskId || !sessionId) return false;
      setIsRunning(true);
      setError(null);
      try {
        const result = await resumeWithHandoff(sessionId, instructions);
        if (!result.sent) {
          onHandoffNotSent?.(result.prompt);
          setError(t("task:resumeHandoffNotSent"));
          return false;
        }
        clearContextWindow(sessionId);
        return true;
      } catch (err: unknown) {
        // Raw backend/utility messages can carry internal identifiers, so they
        // are logged rather than shown; the offer shows stable translated copy.
        // i18n-exempt: operator diagnostic; the user sees translated copy.
        console.error("Resume with handoff failed", err);
        const errorCode = err instanceof ApiError ? err.errorCode : undefined;
        if (errorCode === "extraction_failed") {
          setError(t("task:resumeHandoffCouldNotExtract"));
        } else {
          setError(t("task:resumeHandoffFailed"));
        }
        return false;
      } finally {
        setIsRunning(false);
      }
    },
    [taskId, sessionId, clearContextWindow, onHandoffNotSent],
  );

  return { run, isRunning, error, clearError };
}
