"use client";

import { useCallback, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Textarea } from "@kandev/ui/textarea";
import { Badge } from "@kandev/ui/badge";
import { IconEdit, IconFileText } from "@tabler/icons-react";
import type { ClarificationAnswer, ClarificationApprovalMeta } from "@/lib/types/http";
import { buildApprovalAnswer, canSubmitRevise, type ApprovalDecision } from "@/lib/approval";
import { useApprovalRequest } from "@/hooks/domains/comments/use-approval-request";
import { ClarificationMarkdown } from "./clarification-markdown";

type ApprovalRequestCardProps = {
  approval: ClarificationApprovalMeta;
  summary?: string | null;
  feedback: string;
  onFeedbackChange: (value: string) => void;
  commentCount: number;
  subjectEdited: boolean;
  isSubmitting: boolean;
  onOpenPlan: () => void;
  onDecision: (decision: ApprovalDecision) => void;
};

/**
 * ApprovalRequestCard is the interactive approval surface: title and summary,
 * an Open plan action, the live pending-comment count, an edited indicator, a
 * feedback field, and Approve / Revise / Reject. Pure props so it is testable
 * without a store; {@link ConnectedApprovalRequestCard} supplies the data.
 */
export function ApprovalRequestCard({
  approval,
  summary,
  feedback,
  onFeedbackChange,
  commentCount,
  subjectEdited,
  isSubmitting,
  onOpenPlan,
  onDecision,
}: ApprovalRequestCardProps) {
  const { t } = useTranslation();
  const reviseEnabled = canSubmitRevise({ feedback, commentCount, subjectEdited });

  return (
    <div className="space-y-3" data-testid="approval-request-card">
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0 space-y-1">
          <div className="text-sm font-medium text-foreground">{approval.title}</div>
          <div className="flex flex-wrap items-center gap-1.5 text-[10px] text-muted-foreground">
            <Badge variant="secondary" className="h-auto min-h-5 text-[10px]">
              {t("task:approvalBadge")}
            </Badge>
            {commentCount > 0 ? (
              <span data-testid="approval-comment-count">
                {t("task:approvalCommentsPending", { count: commentCount })}
              </span>
            ) : null}
            {subjectEdited ? (
              <span className="inline-flex items-center gap-0.5" data-testid="approval-edited">
                <IconEdit className="h-3 w-3" />
                {t("task:approvalEditedSinceRequest")}
              </span>
            ) : null}
          </div>
        </div>
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="h-7 shrink-0 cursor-pointer gap-1 text-xs [@media(pointer:coarse)]:min-h-11"
          onClick={onOpenPlan}
          disabled={isSubmitting}
        >
          <IconFileText className="h-3.5 w-3.5" />
          {t("task:approvalOpenPlan")}
        </Button>
      </div>

      {summary ? (
        <ClarificationMarkdown variant="block" className="text-xs text-foreground">
          {summary}
        </ClarificationMarkdown>
      ) : null}

      <Textarea
        value={feedback}
        onChange={(event) => onFeedbackChange(event.target.value)}
        placeholder={t("task:approvalFeedbackPlaceholder")}
        className="min-h-[64px] resize-y text-xs"
        disabled={isSubmitting}
        aria-label={t("task:approvalFeedbackPlaceholder")}
      />

      <div className="flex flex-col gap-2 sm:flex-row sm:justify-end">
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="cursor-pointer [@media(pointer:coarse)]:min-h-11"
          onClick={() => onDecision("reject")}
          disabled={isSubmitting}
        >
          {t("task:approvalReject")}
        </Button>
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="cursor-pointer [@media(pointer:coarse)]:min-h-11"
          onClick={() => onDecision("revise")}
          disabled={isSubmitting || !reviseEnabled}
          title={reviseEnabled ? undefined : t("task:approvalReviseHint")}
        >
          {t("task:approvalRevise")}
        </Button>
        <Button
          type="button"
          size="sm"
          className="cursor-pointer [@media(pointer:coarse)]:min-h-11"
          onClick={() => onDecision("approve")}
          disabled={isSubmitting}
        >
          {t("task:approvalApprove")}
        </Button>
      </div>
    </div>
  );
}

type ConnectedApprovalRequestCardProps = {
  approval: ClarificationApprovalMeta;
  summary?: string | null;
  requestCreatedAt?: string | null;
  isSubmitting: boolean;
  onSubmitAnswer: (answer: ClarificationAnswer) => void | Promise<void>;
};

/**
 * ConnectedApprovalRequestCard owns the approval decision state: it reads the
 * task's pending plan comments and current plan version, tracks the feedback
 * draft, and builds the clarification answer.
 */
export function ConnectedApprovalRequestCard({
  approval,
  summary,
  requestCreatedAt,
  isSubmitting,
  onSubmitAnswer,
}: ConnectedApprovalRequestCardProps) {
  const [feedback, setFeedback] = useState("");
  const { pendingComments, commentCount, subjectEdited, openPlan } =
    useApprovalRequest(requestCreatedAt);

  const onDecision = useCallback(
    (decision: ApprovalDecision) => {
      const answer = buildApprovalAnswer({
        decision,
        feedback,
        comments: decision === "revise" ? pendingComments : undefined,
      });
      void onSubmitAnswer(answer);
    },
    [feedback, pendingComments, onSubmitAnswer],
  );

  return (
    <ApprovalRequestCard
      approval={approval}
      summary={summary}
      feedback={feedback}
      onFeedbackChange={setFeedback}
      commentCount={commentCount}
      subjectEdited={subjectEdited}
      isSubmitting={isSubmitting}
      onOpenPlan={openPlan}
      onDecision={onDecision}
    />
  );
}
