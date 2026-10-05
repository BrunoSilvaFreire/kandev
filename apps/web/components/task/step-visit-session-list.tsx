"use client";

import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";
import { cn } from "@/lib/utils";
import { formatRelativeTime } from "@/lib/utils";
import type { StepVisitSession } from "@/lib/api/domains/task-activity-api";

export type StepVisitSessionListProps = {
  sessions: readonly StepVisitSession[];
  /** Activate a destination session when clicked. */
  onOpenSession?: (sessionId: string) => void;
  /** Stable prefix for the list and row test ids. */
  testIdPrefix: string;
  className?: string;
};

/**
 * One clickable row per destination session of a step visit, shared by the
 * transition summary, the header hover card, and the compact disclosure. Rows
 * name the session (never a bare id), show its agent when known, and a relative
 * arrival time. Coarse pointers get a 44px touch row.
 */
export function StepVisitSessionList({
  sessions,
  onOpenSession,
  testIdPrefix,
  className,
}: StepVisitSessionListProps) {
  const { t } = useTranslation();
  const taskSessions = useAppStore((state) => state.taskSessions.items);
  const profiles = useAppStore((state) => state.agentProfiles.items);
  const profileNameById = useMemo(
    () => new Map(profiles.map((profile) => [profile.id, profile.label])),
    [profiles],
  );

  if (sessions.length === 0) return null;

  return (
    <ul className={cn("space-y-0.5", className)} data-testid={`${testIdPrefix}-list`}>
      {sessions.map((session) => {
        const name = taskSessions[session.session_id]?.name?.trim();
        const label = name || t("task:visitSessionFallback");
        const agent = session.agent_profile_id
          ? profileNameById.get(session.agent_profile_id)
          : undefined;
        return (
          <li key={session.session_id}>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              disabled={!onOpenSession}
              title={session.session_id}
              data-testid={`${testIdPrefix}-${session.session_id}`}
              onClick={() => onOpenSession?.(session.session_id)}
              className="h-auto min-h-7 w-full justify-start gap-2 px-2 py-1 text-left text-xs cursor-pointer [@media(pointer:coarse)]:min-h-11"
            >
              <span className="min-w-0 truncate">{label}</span>
              {agent ? (
                <span className="min-w-0 truncate text-muted-foreground">{agent}</span>
              ) : null}
              <span className="ml-auto shrink-0 tabular-nums text-muted-foreground">
                {formatRelativeTime(session.occurred_at)}
              </span>
            </Button>
          </li>
        );
      })}
    </ul>
  );
}
