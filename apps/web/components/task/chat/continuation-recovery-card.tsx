"use client";

import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Badge } from "@kandev/ui/badge";
import type { ClarificationAnswer, ClarificationContinuationRecoveryMeta } from "@/lib/types/http";
import { ClarificationMarkdown } from "./clarification-markdown";

const REASON_KEYS: Record<ClarificationContinuationRecoveryMeta["reason"], string> = {
  extraction_failed: "task:continuationRecoveryReasonExtractionFailed",
  reset_failed: "task:continuationRecoveryReasonResetFailed",
  no_available_profile: "task:continuationRecoveryReasonNoAvailableProfile",
};

type ContinuationRecoveryCardProps = {
  recovery: ClarificationContinuationRecoveryMeta;
  questionId: string;
  summary?: string | null;
  isSubmitting: boolean;
  onSubmitAnswer: (answer: ClarificationAnswer) => void | Promise<void>;
};

/**
 * ContinuationRecoveryCard is the decision surface for a paused automatic
 * handoff (D10): the user either retries the handoff or continues without one.
 * The bundle is system-authored, so the card supplies the localized copy for
 * the fixed question instead of rendering the server's English labels.
 */
export function ContinuationRecoveryCard({
  recovery,
  questionId,
  summary,
  isSubmitting,
  onSubmitAnswer,
}: ContinuationRecoveryCardProps) {
  const { t } = useTranslation();
  const decide = (decision: "retry" | "continue") => {
    void onSubmitAnswer({ question_id: questionId, selected_options: [decision] });
  };

  return (
    <div className="space-y-3" data-testid="continuation-recovery-card">
      <div className="flex items-center gap-1.5 text-[10px] text-muted-foreground">
        <Badge variant="secondary" className="h-auto min-h-5 text-[10px]">
          {t("task:continuationRecoveryBadge")}
        </Badge>
      </div>
      <div className="text-sm font-medium text-foreground">
        {t("task:continuationRecoveryTitle")}
      </div>
      <p className="text-xs text-muted-foreground">{t(REASON_KEYS[recovery.reason])}</p>
      {summary ? (
        <ClarificationMarkdown variant="block" className="text-xs text-foreground">
          {summary}
        </ClarificationMarkdown>
      ) : null}
      <div className="flex flex-col gap-2 sm:flex-row sm:justify-end">
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="cursor-pointer [@media(pointer:coarse)]:min-h-11"
          onClick={() => decide("continue")}
          disabled={isSubmitting}
        >
          {t("task:continuationRecoveryContinue")}
        </Button>
        <Button
          type="button"
          size="sm"
          className="cursor-pointer [@media(pointer:coarse)]:min-h-11"
          onClick={() => decide("retry")}
          disabled={isSubmitting}
        >
          {t("task:continuationRecoveryRetry")}
        </Button>
      </div>
    </div>
  );
}
