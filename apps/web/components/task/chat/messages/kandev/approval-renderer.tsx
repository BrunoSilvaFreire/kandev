"use client";

import { IconFileCheck } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { EmptyListNote, KandevBody, KandevRow, KeyValueRow } from "./shared";
import { pickString } from "./parse";
import type { KandevRenderer } from "./types";
import { useTranslation } from "react-i18next";
import { ClarificationMarkdown } from "../../clarification-markdown";

function pickObject(value: unknown, key: string): Record<string, unknown> | undefined {
  if (!value || typeof value !== "object") return undefined;
  const entry = (value as Record<string, unknown>)[key];
  return entry && typeof entry === "object" ? (entry as Record<string, unknown>) : undefined;
}

type ApprovalFields = {
  subject?: string;
  title?: string;
  summary?: string;
  decision?: string;
  feedback?: string;
  planComments?: string;
  subjectEdited: boolean;
};

/** Reads the request args and the (possibly nested) recorded outcome. */
function readApprovalFields(args: Record<string, unknown> | undefined, result: unknown): ApprovalFields {
  const approval = pickObject(result, "approval");
  return {
    subject: pickString(args, "subject"),
    title: pickString(args, "title"),
    summary: pickString(args, "summary") ?? pickString(args, "context"),
    decision: pickString(approval, "decision") ?? pickString(result, "decision"),
    feedback: pickString(approval, "feedback") ?? pickString(result, "feedback"),
    planComments: pickString(approval, "plan_comments") ?? pickString(result, "plan_comments"),
    subjectEdited: approval?.subject_edited === true,
  };
}

/**
 * ApprovalRenderer renders the transcript row for request_approval_kandev: the
 * subject and summary the agent asked about, and the recorded decision once the
 * user answered. Answering itself happens in the clarification overlay.
 */
export const ApprovalRenderer: KandevRenderer = ({ args, result, status }) => {
  const { t } = useTranslation();
  const { subject, title, summary, decision, feedback, planComments, subjectEdited } =
    readApprovalFields(args, result);
  const summaryText = title ?? subject;
  const hasBody = Boolean(summary || decision);

  return (
    <div data-testid="approval-renderer">
      <KandevRow
        Icon={IconFileCheck}
        title={t("task:kandevRequestApproval")}
        summary={
          summaryText ? (
            <span className="truncate">
              <ClarificationMarkdown variant="inline" linkBehavior="passive" className="inline">
                {summaryText}
              </ClarificationMarkdown>
            </span>
          ) : undefined
        }
        status={status}
        hasExpandableContent={hasBody}
      >
        <KandevBody>
          {subject ? (
            <KeyValueRow label={t("task:approvalSubjectLabel")}>
              <Badge variant="outline" className="h-auto min-h-5 text-[10px]">
                {subject}
              </Badge>
            </KeyValueRow>
          ) : null}
          {summary ? (
            <ClarificationMarkdown variant="block" className="text-xs text-foreground">
              {summary}
            </ClarificationMarkdown>
          ) : null}
          {decision ? (
            <KeyValueRow label={t("task:approvalDecisionLabel")}>
              <span>
                {decision}
                {subjectEdited ? ` (${t("task:approvalEditedSinceRequest")})` : ""}
              </span>
            </KeyValueRow>
          ) : null}
          {feedback ? (
            <KeyValueRow label={t("task:approvalFeedbackLabel")}>
              <span className="whitespace-pre-wrap">{feedback}</span>
            </KeyValueRow>
          ) : null}
          {planComments ? (
            <ClarificationMarkdown variant="block" className="text-xs text-muted-foreground">
              {planComments}
            </ClarificationMarkdown>
          ) : null}
          {!hasBody ? <EmptyListNote messageKey="task:approvalAwaitingResponse" /> : null}
        </KandevBody>
      </KandevRow>
    </div>
  );
};
