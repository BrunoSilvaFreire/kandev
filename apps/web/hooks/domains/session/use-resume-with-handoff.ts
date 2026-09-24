import { useCallback, useEffect, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { runTranscriptUtility } from "@/hooks/use-summarize-session";
import { sendMessageRequest } from "@/hooks/message-request";
import { requestContextReset } from "@/lib/api/domains/session-reset-api";
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
  newestMessageAt: number | null;
  now: number;
};

/**
 * Pure eligibility check for the resume-with-handoff offer. True only for a
 * settled, idle session with a live agent, an agent message, and a newest
 * message older than the cache-expiry window.
 */
export function isResumeHandoffCandidate(input: ResumeHandoffCandidateInput): boolean {
  if (input.state !== "WAITING_FOR_INPUT") return false;
  if (input.isAgentBusy) return false;
  if (!input.hasLiveExecution) return false;
  if (!input.hasAgentMessage) return false;
  if (input.newestMessageAt === null) return false;
  return input.now - input.newestMessageAt >= CACHE_EXPIRY_MS;
}

export type ResumeWithHandoffOptions = {
  /**
   * Called with the extracted handoff when the reset succeeded but the send
   * failed, so the caller can place it in the composer instead of losing it.
   */
  onHandoffNotSent?: (handoff: string) => void;
};

export type ResumeWithHandoffResult = {
  run: () => Promise<void>;
  isRunning: boolean;
  error: string | null;
  clearError: () => void;
};

/**
 * Extracts a facts-only handoff from the session transcript, resets the live
 * agent's context, and sends the handoff as the first prompt of the fresh
 * context. Extraction runs first, so a failed or empty extraction leaves the
 * existing context untouched. Nothing is cached between runs: a retry always
 * re-extracts and re-resets, so a later click can never reuse a handoff or a
 * reset from a different session or an earlier point in this one.
 */
export function useResumeWithHandoff(
  taskId: string | null,
  sessionId: string | null,
  options: ResumeWithHandoffOptions = {},
): ResumeWithHandoffResult {
  const [isRunning, setIsRunning] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const clearContextWindow = useAppStore((state) => state.clearContextWindow);
  const addMessage = useAppStore((state) => state.addMessage);
  const { onHandoffNotSent } = options;

  // The composer is not remounted when the active session changes, so a
  // previous session's failure must not surface on the next one.
  useEffect(() => {
    setError(null);
  }, [sessionId]);

  const clearError = useCallback(() => setError(null), []);

  const run = useCallback(async () => {
    if (!taskId || !sessionId) return;
    setIsRunning(true);
    setError(null);
    let handoff: string | undefined;
    let resetDone = false;
    try {
      const extraction = await runTranscriptUtility(sessionId, RESUME_HANDOFF_AGENT_ID);
      handoff = extraction.status === "ok" ? extraction.text?.trim() : undefined;
      if (!handoff) {
        // i18n-exempt: operator diagnostic; the user sees translated copy.
        console.error("Resume handoff extraction produced no text", extraction);
        setError(t("task:resumeHandoffCouldNotExtract"));
        return;
      }
      await requestContextReset(sessionId);
      resetDone = true;
      clearContextWindow(sessionId);
      const created = await sendMessageRequest({
        taskId,
        resolvedSessionId: sessionId,
        finalMessage: handoff,
        modelToSend: undefined,
        planMode: false,
      });
      if (created?.id && created.session_id) addMessage(created);
    } catch (err) {
      // Raw backend/utility messages can carry internal identifiers, so they
      // are logged rather than shown; the offer shows stable translated copy.
      // i18n-exempt: operator diagnostic; the user sees translated copy.
      console.error("Resume with handoff failed", err);
      if (resetDone && handoff) {
        onHandoffNotSent?.(handoff);
        setError(t("task:resumeHandoffNotSent"));
      } else {
        setError(t("task:resumeHandoffFailed"));
      }
    } finally {
      setIsRunning(false);
    }
  }, [taskId, sessionId, clearContextWindow, addMessage, onHandoffNotSent]);

  return { run, isRunning, error, clearError };
}
