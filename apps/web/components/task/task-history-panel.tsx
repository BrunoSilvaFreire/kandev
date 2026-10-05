"use client";

import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";
import { IconFileText, IconLoader2, IconRoute, IconSparkles } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";
import { formatRelativeTime } from "@/lib/utils";
import { useTaskActivity } from "@/hooks/domains/task/use-task-activity";
import type { TaskActivityEvent } from "@/lib/api/domains/task-activity-api";
import {
  rawHistoryFields,
  routeExplanation,
  transitionExplanation,
  type HistoryField,
  type HistoryNames,
} from "./task-history-fields";

type TaskHistoryPanelProps = {
  taskId: string | null;
  /** Optional destination-step filter (step visits only). */
  stepId?: string | null;
  onOpenSession?: (sessionId: string) => void;
  onReview?: (documentKey: string) => void;
};

/** Resolve a step id to its display name from the workflow store; the id is
 *  the authority and unresolved (removed) steps stay unlabeled. */
function useStepName(stepId: string | null | undefined): string | null {
  return useAppStore((state) => {
    if (!stepId) return null;
    const active = state.kanban.steps.find((step) => step.id === stepId);
    if (active) return active.title;
    for (const snapshot of Object.values(state.kanbanMulti.snapshots)) {
      const match = snapshot?.steps.find((step) => step.id === stepId);
      if (match) return match.title;
    }
    return null;
  });
}

/** Resolve an agent profile id to its human label; id is never the label. */
function useProfileLabel(profileId: string | null | undefined): string | null {
  return useAppStore((state) => {
    if (!profileId) return null;
    return state.agentProfiles.items.find((profile) => profile.id === profileId)?.label ?? null;
  });
}

/** Resolve a session id to its title when the session still exists. */
function useSessionLabel(sessionId: string | null | undefined): string | null {
  return useAppStore((state) => {
    if (!sessionId) return null;
    return state.taskSessions.items[sessionId]?.name ?? null;
  });
}

function historyRowTitle(t: TFunction, event: TaskActivityEvent, stepName: string | null): string {
  switch (event.kind) {
    case "transition":
      return stepName
        ? t("task:taskHistoryStepMoved", { step: stepName })
        : t("task:taskHistoryStepMovedUnknown");
    case "session_created":
      return t("task:taskHistorySessionCreated");
    case "session_completed":
      return t("task:taskHistorySessionCompleted");
    case "route":
      return t("task:taskHistoryRoute");
    case "document_revision":
      return t("task:taskHistoryDocumentRevision", {
        key: event.document_revision?.document_key ?? "",
        number: event.document_revision?.revision_number ?? 0,
      });
    case "review_run":
      return t("task:taskHistoryReviewRun");
    default:
      return event.kind;
  }
}

function historyIcon(kind: TaskActivityEvent["kind"]): React.ReactNode {
  if (kind === "route") return <IconRoute className="h-3.5 w-3.5" />;
  if (kind === "document_revision") return <IconFileText className="h-3.5 w-3.5" />;
  return <IconSparkles className="h-3.5 w-3.5" />;
}

/** Detail line + the session a row may activate (destination only for routes). */
function historyDetail(
  t: TFunction,
  event: TaskActivityEvent,
  names: HistoryNames,
): { detail: string | null; sessionId: string | null } {
  switch (event.kind) {
    case "transition":
      return event.route
        ? {
            detail: routeExplanation(t, event, names),
            sessionId: event.route.destination_session_id ?? null,
          }
        : { detail: transitionExplanation(t, event), sessionId: null };
    case "session_created":
    case "session_completed":
      return sessionDetail(t, event);
    case "route":
      return {
        detail: routeExplanation(t, event, names),
        sessionId: event.route?.destination_session_id ?? null,
      };
    case "review_run":
      return reviewDetail(t, event);
    default:
      return { detail: null, sessionId: null };
  }
}

function sessionDetail(
  t: TFunction,
  event: TaskActivityEvent,
): { detail: string | null; sessionId: string | null } {
  const session = event.session;
  return {
    detail: session?.name || session?.agent_profile_id || t("task:taskHistoryUnknownSession"),
    sessionId: session?.id ?? null,
  };
}

function reviewDetail(
  t: TFunction,
  event: TaskActivityEvent,
): { detail: string | null; sessionId: string | null } {
  return {
    detail: event.review_run
      ? t("task:taskHistoryReviewStatus", { status: event.review_run.status })
      : null,
    sessionId: null,
  };
}

function HistoryDetails({ fields }: { fields: HistoryField[] }) {
  const { t } = useTranslation();
  if (fields.length === 0) {
    return (
      <p className="mt-1 text-[0.6875rem] text-muted-foreground">{t("task:taskHistoryNoDetail")}</p>
    );
  }
  return (
    <dl className="mt-1 grid grid-cols-[auto_1fr] gap-x-2 gap-y-0.5 text-[0.6875rem] text-muted-foreground">
      {fields.map((field) => (
        <div key={field.label} className="contents">
          <dt>{field.label}</dt>
          <dd className="break-all">{field.value}</dd>
        </div>
      ))}
    </dl>
  );
}

function HistoryRowActions({
  event,
  sessionId,
  onOpenSession,
  onReview,
}: {
  event: TaskActivityEvent;
  sessionId: string | null;
  onOpenSession?: (sessionId: string) => void;
  onReview?: (documentKey: string) => void;
}) {
  const { t } = useTranslation();
  const reviewKey =
    event.kind === "document_revision" ? event.document_revision?.document_key : null;
  return (
    <>
      {reviewKey && onReview && (
        <Button
          variant="ghost"
          size="sm"
          className="h-6 cursor-pointer px-2 text-xs [@media(pointer:coarse)]:h-11 [@media(pointer:coarse)]:min-w-11"
          data-testid={`task-history-review-${reviewKey}`}
          onClick={() => onReview(reviewKey)}
        >
          {t("task:review")}
        </Button>
      )}
      {sessionId && onOpenSession && (
        <Button
          variant="ghost"
          size="sm"
          className="h-6 cursor-pointer px-2 text-xs [@media(pointer:coarse)]:h-11 [@media(pointer:coarse)]:min-w-11"
          data-testid={`task-history-open-${event.id}`}
          onClick={() => onOpenSession(sessionId)}
        >
          {t("task:searchOpenSession")}
        </Button>
      )}
    </>
  );
}

function HistoryRow({
  event,
  onOpenSession,
  onReview,
}: {
  event: TaskActivityEvent;
  onOpenSession?: (sessionId: string) => void;
  onReview?: (documentKey: string) => void;
}) {
  const { t } = useTranslation();
  const stepName = useStepName(event.transition?.to_step_id ?? event.route?.destination_step_id);
  const profileName = useProfileLabel(event.route?.agent_profile_id);
  const sourceSessionName = useSessionLabel(event.route?.source_session_id);
  const destinationSessionName = useSessionLabel(event.route?.destination_session_id);
  const names: HistoryNames = { stepName, profileName, sourceSessionName, destinationSessionName };
  const title = historyRowTitle(t, event, stepName);
  const time = formatRelativeTime(event.occurred_at);
  const { detail, sessionId } = historyDetail(t, event, names);
  const fields = rawHistoryFields(t, event, names);

  return (
    <li
      className="flex items-start gap-2 border-b border-border/60 px-3 py-2 last:border-0"
      data-testid={`task-history-row-${event.kind}`}
      data-event-id={event.id}
    >
      <span className="mt-0.5 shrink-0 text-muted-foreground" aria-hidden="true">
        {historyIcon(event.kind)}
      </span>
      <div className="min-w-0 flex-1">
        <p className="truncate text-xs font-medium">{title}</p>
        {detail && <p className="truncate text-xs text-muted-foreground">{detail}</p>}
        {(event.route || event.transition) && (
          <details data-testid="task-history-details">
            <summary className="cursor-pointer text-[0.6875rem] text-muted-foreground">
              {t("task:taskHistoryDetails")}
            </summary>
            <HistoryDetails fields={fields} />
          </details>
        )}
      </div>
      <span className="shrink-0 text-[0.6875rem] text-muted-foreground">{time}</span>
      <HistoryRowActions
        event={event}
        sessionId={sessionId}
        onOpenSession={onOpenSession}
        onReview={onReview}
      />
    </li>
  );
}

/** Cursor-paginated durable task activity timeline. */
export function TaskHistoryPanel({
  taskId,
  stepId = null,
  onOpenSession,
  onReview,
}: TaskHistoryPanelProps) {
  const { t } = useTranslation();
  const activity = useTaskActivity(taskId, { stepId });

  if (activity.status === "loading" && activity.events.length === 0) {
    return (
      <div className="flex h-full items-center justify-center text-muted-foreground">
        <IconLoader2 className="mr-2 h-4 w-4 animate-spin" />
        <span className="text-sm">{t("task:loading")}</span>
      </div>
    );
  }

  if (activity.status === "error" && activity.events.length === 0) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-3 text-muted-foreground">
        <span className="text-sm">{t("task:taskHistoryLoadFailed")}</span>
        <Button variant="outline" size="sm" className="cursor-pointer" onClick={activity.reload}>
          {t("task:retry")}
        </Button>
      </div>
    );
  }

  if (activity.events.length === 0) {
    return (
      <div className="flex h-full items-center justify-center text-muted-foreground">
        <span className="text-sm">{t("task:taskHistoryEmpty")}</span>
      </div>
    );
  }

  return (
    <div className="flex h-full min-h-0 flex-col" data-testid="task-history-panel">
      <ul className="min-h-0 flex-1 overflow-y-auto" data-testid="task-history-rows">
        {activity.events.map((event) => (
          <HistoryRow
            key={`${event.kind}:${event.id}`}
            event={event}
            onOpenSession={onOpenSession}
            onReview={onReview}
          />
        ))}
      </ul>
      {activity.hasMore && (
        <div className="shrink-0 border-t border-border/60 p-2">
          <Button
            variant="ghost"
            size="sm"
            className="w-full cursor-pointer text-xs"
            data-testid="task-history-load-more"
            onClick={activity.loadMore}
          >
            {t("task:taskHistoryLoadMore")}
          </Button>
        </div>
      )}
    </div>
  );
}
