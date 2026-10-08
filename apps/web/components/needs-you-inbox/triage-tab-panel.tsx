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
import type { InboxTriageItem, InboxTriageKind } from "@/lib/inbox-triage/possible-question-triage";

const KIND_LABEL_KEYS: Record<InboxTriageKind, string> = {
  clarification: "needsYouInbox:triageKindClarification",
  stale_review: "needsYouInbox:triageKindStaleReview",
  possible_question: "needsYouInbox:triageKindPossibleQuestion",
};

const KIND_BADGE_VARIANT: Record<InboxTriageKind, "default" | "secondary" | "outline"> = {
  clarification: "default",
  stale_review: "outline",
  possible_question: "secondary",
};

function triageHref(item: InboxTriageItem): string {
  return linkToTask(item.taskId, { sessionId: item.sessionId ?? undefined });
}

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
        {item.updatedAt && (
          <span className="text-xs text-muted-foreground">
            {formatRelativeTime(item.updatedAt)}
          </span>
        )}
      </span>
      <Badge variant={KIND_BADGE_VARIANT[item.kind]} className="shrink-0">
        {t(KIND_LABEL_KEYS[item.kind])}
      </Badge>
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
 * The triage lane beside the clarification-backed Needs-you Inbox: real
 * clarifications first, then stale or interrupted Review tasks, then advisory
 * possible-question hints. Viewing the lane acknowledges the surfaced
 * identities so the in-app nudge fires once per new item.
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
