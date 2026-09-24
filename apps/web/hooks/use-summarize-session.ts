import { useCallback, useState } from "react";
import { executeUtilityPrompt } from "@/lib/api/domains/utility-api";
import { listTaskSessionMessages } from "@/lib/api/domains/session-api";
import type { Message } from "@/lib/types/http";
import { t } from "@/lib/i18n";

export type SummarizeSessionResult = {
  summary: string | null;
  error?: string;
};

export type TranscriptUtilityResult =
  | { status: "empty" }
  | { status: "failed"; error?: string }
  | { status: "ok"; text: string | null };

function formatTranscript(messages: Message[]): string {
  return messages
    .filter((m) => m.type === "message" || m.type === "content")
    .map((m) => {
      // i18n-exempt: transcript role markers sent verbatim to the utility model, not shown to a user.
      const role = m.author_type === "user" ? "User" : "Agent";
      return `${role}: ${m.content}`;
    })
    .join("\n\n");
}

/**
 * Fetches a session's transcript and runs it through a transcript-consuming
 * utility agent. Network and transport failures throw; a utility that ran but
 * did not succeed resolves as `failed`.
 */
export async function runTranscriptUtility(
  sessionId: string,
  utilityAgentId: string,
): Promise<TranscriptUtilityResult> {
  // Fetch messages from API; they may not be in the store for non-active sessions.
  const resp = await listTaskSessionMessages(sessionId, { sort: "asc" });
  const messages = resp.messages ?? [];
  if (!messages.length) return { status: "empty" };

  const transcript = formatTranscript(messages);
  if (!transcript) return { status: "empty" };

  const result = await executeUtilityPrompt({
    utility_agent_id: utilityAgentId,
    conversation_history: transcript,
  });
  if (!result.success) {
    return { status: "failed", error: result.error };
  }
  return { status: "ok", text: result.response ?? null };
}

export function useSummarizeSession() {
  const [isSummarizing, setIsSummarizing] = useState(false);

  const summarize = useCallback(async (sessionId: string): Promise<SummarizeSessionResult> => {
    setIsSummarizing(true);
    try {
      // Sessionless: handoff often runs against a completed session whose
      // agentctl is gone. Host utility executes the builtin summarize agent.
      const result = await runTranscriptUtility(sessionId, "builtin-summarize-session");
      if (result.status === "empty") return { summary: null };
      if (result.status === "failed") {
        return { summary: null, error: result.error || t("task:summarizeReturnedNoResult") };
      }
      return { summary: result.text };
    } catch (error) {
      return {
        summary: null,
        error: error instanceof Error ? error.message : t("task:couldNotGenerateSummary"),
      };
    } finally {
      setIsSummarizing(false);
    }
  }, []);

  return { summarize, isSummarizing };
}
