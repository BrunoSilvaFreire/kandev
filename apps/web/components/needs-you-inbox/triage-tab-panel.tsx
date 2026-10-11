"use client";

import { useEffect } from "react";
import { useTranslation } from "react-i18next";
import { Badge } from "@kandev/ui/badge";
import Link from "@/components/routing/app-link";
import { linkToTask } from "@/lib/links";
import { formatRelativeTime } from "@/lib/i18n/formats";
import {
  useInboxTriageNudge,
  type InboxTriageNudge,
} from "@/hooks/domains/inbox-triage/use-inbox-triage";
import type {
  InboxTriageItem,
  InboxTriageKind,
  InboxTriageReason,
} from "@/lib/inbox-triage/possible-question-triage";

const KIND_LABEL_KEYS: Record<InboxTriageKind, string> = {
  clarification: "needsYouInbox:triageKindClarification",
  interrupted: "needsYouInbox:triageKindInterrupted",
  stale_review: "needsYouInbox:triageKindStaleReview",
  review: "needsYouInbox:triageKindReview",
  dependency: "needsYouInbox:triageKindDependency",
  possible_question: "needsYouInbox:triageKindPossibleQuestion",
};

const KIND_ACTION_KEYS: Record<InboxTriageKind, string> = {
  clarification: "needsYouInbox:triageActionAnswer",
  interrupted: "needsYouInbox:triageActionResume",
  stale_review: "needsYouInbox:triageActionReview",
  review: "needsYouInbox:triageActionReview",
  dependency: "needsYouInbox:triageActionOpen",
  possible_question: "needsYouInbox:triageActionOpen",
};

const REASON_KEYS: Record<InboxTriageReason, string> = {
  clarification: "needsYouInbox:triageReasonClarification",
  backend_restart: "needsYouInbox:triageReasonBackendRestart",
  execution_lost: "needsYouInbox:triageReasonExecutionLost",
  interrupted_unknown: "needsYouInbox:triageReasonInterruptedUnknown",
  stale_review: "needsYouInbox:triageReasonStaleReview",
  review: "needsYouInbox:triageReasonReview",
  dependency: "needsYouInbox:triageReasonDependency",
  possible_question: "needsYouInbox:triageReasonPossibleQuestion",
};

const KIND_BADGE_VARIANT: Record<InboxTriageKind, "default" | "secondary" | "outline"> = {
  clarification: "default",
  interrupted: "default",
  stale_review: "outline",
  review: "outline",
  dependency: "secondary",
  possible_question: "secondary",
};

function triageHref(item: InboxTriageItem): string {
  return linkToTask(item.taskId, { sessionId: item.sessionId ?? undefined });
}

// The whole row is the action: one touch-sized link whose accessible name
// carries the task, the reason, and what opening it lets the operator do.
function TriageRow({ item }: { item: InboxTriageItem }) {
  const { t } = useTranslation();
  return (
    <Link
      href={triageHref(item)}
      className="flex cursor-pointer items-center justify-between gap-3 px-4 py-3 hover:bg-accent/50 max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
      data-testid={`inbox-triage-row-${item.kind}`}
    >
      <span className="flex min-w-0 flex-col gap-1">
        <span className="truncate text-sm text-foreground">{item.title}</span>
        <span className="text-xs text-muted-foreground" data-testid="inbox-triage-reason">
          {t(REASON_KEYS[item.reason])}
          {item.since && ` · ${formatRelativeTime(item.since)}`}
        </span>
      </span>
      <span className="flex shrink-0 flex-col items-end gap-1 md:flex-row md:items-center md:gap-2">
        <Badge variant={KIND_BADGE_VARIANT[item.kind]}>{t(KIND_LABEL_KEYS[item.kind])}</Badge>
        <span className="text-xs font-medium text-foreground" data-testid="inbox-triage-action">
          {t(KIND_ACTION_KEYS[item.kind])}
        </span>
      </span>
    </Link>
  );
}

function TriageList({ items }: { items: readonly InboxTriageItem[] }) {
  const { t } = useTranslation();
  return (
    <div
      className="overflow-hidden rounded-lg border border-border divide-y divide-border"
      data-testid="inbox-triage-list"
    >
      {items.map((item) => (
        <TriageRow key={item.id} item={item} />
      ))}
      <p className="px-4 py-3 text-xs text-muted-foreground" data-testid="inbox-triage-caption">
        {t("needsYouInbox:triageCaption")}
      </p>
    </div>
  );
}

/**
 * The triage lane beside the clarification-backed Needs-you Inbox: one row per
 * task with its reason and next action. Viewing the lane acknowledges the
 * surfaced identities so the in-app nudge fires once per new incident.
 */
export function TriageTabPanel({ controller }: { controller?: InboxTriageNudge }) {
  const { t } = useTranslation();
  const fallback = useInboxTriageNudge();
  const { items, acknowledgeAll } = controller ?? fallback;

  useEffect(() => {
    acknowledgeAll();
  }, [acknowledgeAll]);

  if (items.length === 0) {
    return (
      <p className="text-sm text-muted-foreground" data-testid="inbox-triage-empty">
        {t("needsYouInbox:triageEmpty")}
      </p>
    );
  }

  return <TriageList items={items} />;
}
