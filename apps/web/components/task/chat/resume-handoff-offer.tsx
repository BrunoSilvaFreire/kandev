"use client";

import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { GridSpinner } from "@/components/grid-spinner";
import { useAppStore } from "@/components/state-provider";
import { deriveSessionFlags } from "@/hooks/domains/session/use-session-state";
import {
  isResumeHandoffCandidate,
  useResumeWithHandoff,
} from "@/hooks/domains/session/use-resume-with-handoff";
import { useIsUtilityConfigured } from "@/hooks/use-is-utility-configured";
import { useSessionUsageInspector } from "@/hooks/domains/session/use-session-usage-inspector";
import { CACHE_RECHECK_MS, epochMillisFromWire } from "@/lib/usage/efficiency";
import { newestMessage } from "@/lib/usage/newest-message";
import type { Message } from "@/lib/types/http";

const EMPTY_MESSAGES: Message[] = [];

type ResumeHandoffOfferProps = {
  taskId: string | null;
  sessionId: string | null;
  /** Receives the extracted handoff when the send failed after a reset. */
  onHandoffNotSent?: (handoff: string) => void;
  getDraft?: () => string;
  clearDraft?: () => void;
};

function isConversationMessage(message: Message): boolean {
  return message.type === "message" || message.type === "content";
}

function useResumeHandoffEligibility(taskId: string | null, sessionId: string | null, now: number) {
  const session = useAppStore((state) =>
    sessionId ? (state.taskSessions.items[sessionId] ?? null) : null,
  );
  const messages = useAppStore((state) =>
    sessionId ? (state.messages.bySession[sessionId] ?? EMPTY_MESSAGES) : EMPTY_MESSAGES,
  );
  const agentctlStatus = useAppStore((state) =>
    sessionId ? state.sessionAgentctl.itemsBySessionId[sessionId]?.status : undefined,
  );
  // Cache warmth is keyed on the newest usage-ledger event, so the offer reads
  // the same session totals the usage inspector loads rather than messages.
  const { session: usageTotals } = useSessionUsageInspector(taskId, sessionId, { enabled: true });

  const newest = useMemo(() => newestMessage(messages), [messages]);
  const lastUsageEventAt = epochMillisFromWire(usageTotals?.last_event_at);
  const hasAgentMessage = useMemo(
    () =>
      messages.some((message) => message.author_type !== "user" && isConversationMessage(message)),
    [messages],
  );
  const flags = deriveSessionFlags(session);
  // A pending clarification or permission prompt is a live turn the user must
  // answer, so treat it like a busy agent rather than offering a fresh start.
  const isAgentBusy = flags.isAgentBusy || !!session?.pending_action;
  // An agentctl "error" entry means the agent process is known to be dead, so
  // reset_context would reject. A missing entry is unknown (WS status is not
  // hydrated for idle sessions), so it must not hide the offer.
  const hasLiveExecution = !!sessionId && !isAgentBusy && agentctlStatus !== "error";
  const isCandidate = isResumeHandoffCandidate({
    state: session?.state,
    isAgentBusy,
    hasLiveExecution,
    hasAgentMessage,
    lastUsageEventAt,
    now,
  });
  return {
    isCandidate,
    dismissalKey: sessionId && newest ? `${sessionId}:${newest.id}` : null,
  };
}

export function ResumeHandoffOffer({
  taskId,
  sessionId,
  onHandoffNotSent,
  getDraft,
  clearDraft,
}: ResumeHandoffOfferProps) {
  const { t } = useTranslation();
  const isUtilityConfigured = useIsUtilityConfigured();
  const { run, isRunning, error, clearError } = useResumeWithHandoff(taskId, sessionId, {
    onHandoffNotSent,
  });
  const [dismissedKey, setDismissedKey] = useState<string | null>(null);
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), CACHE_RECHECK_MS);
    return () => clearInterval(timer);
  }, []);

  const { isCandidate, dismissalKey } = useResumeHandoffEligibility(taskId, sessionId, now);
  const isDismissed = dismissalKey !== null && dismissedKey === dismissalKey;
  const draft = getDraft ? getDraft().trim() : "";
  const hasDraft = draft.length > 0;

  // Without a configured utility agent the extraction would always fail, so
  // the offer would only re-arm on every new message.
  if (!isUtilityConfigured) return null;
  // Keep the offer (and its error) mounted after a failure: a successful reset
  // posts a context-reset message that makes the session ineligible. The
  // action stays disabled while ineligible so it cannot pay for an extraction
  // that the reset would reject (for example once the agent is RUNNING again).
  if (!isRunning && !error && (!isCandidate || isDismissed)) {
    return null;
  }

  return (
    <div
      data-testid="resume-handoff-offer"
      className="flex flex-col gap-2 rounded border border-border bg-muted/30 px-3 py-2"
    >
      <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex min-w-0 flex-1 flex-col gap-0.5">
          <span className="text-sm text-muted-foreground">{t("task:resumeHandoffOffer")}</span>
          {hasDraft ? (
            <span
              data-testid="resume-handoff-draft-hint"
              className="text-xs text-muted-foreground/80"
            >
              {t("task:resumeHandoffIncludesDraft")}
            </span>
          ) : null}
        </div>
        <div className="flex w-full flex-col gap-2 sm:w-auto sm:flex-row">
          <Button
            type="button"
            variant="default"
            data-testid="resume-handoff-action"
            className="min-h-11 w-full shrink-0 gap-1.5 cursor-pointer sm:min-h-7 sm:w-auto"
            onClick={async () => {
              const text = getDraft ? getDraft() : "";
              const success = await run(text);
              if (success) {
                clearDraft?.();
              }
            }}
            disabled={isRunning || !isCandidate}
          >
            {isRunning ? <GridSpinner className="h-3.5 w-3.5" /> : null}
            {t("task:resumeHandoffAction")}
          </Button>
          <Button
            type="button"
            variant="outline"
            data-testid="resume-handoff-dismiss"
            className="min-h-11 w-full shrink-0 cursor-pointer sm:min-h-7 sm:w-auto"
            onClick={() => {
              setDismissedKey(dismissalKey);
              clearError();
            }}
            disabled={isRunning}
          >
            {t("task:dismiss")}
          </Button>
        </div>
      </div>
      {error ? (
        <span data-testid="resume-handoff-error" className="text-sm text-destructive">
          {error}
        </span>
      ) : null}
    </div>
  );
}
