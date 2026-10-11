"use client";

import { IconAlertTriangle } from "@tabler/icons-react";
import { Alert, AlertDescription, AlertTitle } from "@kandev/ui/alert";
import { useTranslation } from "react-i18next";
import {
  getTransitionFailure,
  transitionFailureHeadlineKey,
  type TransitionFailureRecord,
} from "@/lib/tasks/transition-failure";
import { IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";
import { useOpenTaskPlan } from "@/hooks/domains/task/use-task-transition-failure";
import { getTaskMoveErrorDetail } from "./task-move-error-message";
import { TransitionFailureDetails } from "./transition-failure-details";

type TaskMoveErrorBannerProps = {
  error: unknown;
};

export function TaskMoveErrorBanner({ error }: TaskMoveErrorBannerProps) {
  const { t } = useTranslation();
  const openPlan = useOpenTaskPlan();
  const title = t("task:failedToMoveTask");
  const detail = getTaskMoveErrorDetail(error, title, t);
  const failure = getTransitionFailure(error);

  return (
    <div className="px-3 pt-2" data-testid="task-move-error-banner">
      <Alert variant="destructive" role="alert">
        <IconAlertTriangle />
        <AlertTitle>{title}</AlertTitle>
        {(detail !== null || failure) && (
          <AlertDescription>
            {detail}
            {failure && <TransitionFailureDetails failure={failure} onOpenPlan={openPlan} />}
          </AlertDescription>
        )}
      </Alert>
    </div>
  );
}

/**
 * A transition that failed after its move or completion signal was accepted.
 * It stays until a later transition of the task succeeds.
 */
export function TaskTransitionFailureBanner({ record }: { record: TransitionFailureRecord }) {
  const { t } = useTranslation();
  const openPlan = useOpenTaskPlan();
  const headlineKey = transitionFailureHeadlineKey(record.failure.code);
  return (
    <div className="px-3 pt-2" data-testid="task-transition-failure-banner">
      <Alert variant="destructive" role="alert">
        <IconAlertTriangle />
        <AlertTitle>
          {record.source === "deferred_move"
            ? t("task:transitionFailureDeferredMove")
            : t("task:transitionFailureAutoAdvance")}
        </AlertTitle>
        <AlertDescription>
          {t(headlineKey ?? "task:taskMoveErrorGeneric")}
          <TransitionFailureDetails failure={record.failure} onOpenPlan={openPlan} />
        </AlertDescription>
      </Alert>
    </div>
  );
}

/**
 * App-wide notice for a refused move reported from a toast-only surface. It
 * stays until dismissed so the reasons outlive the toast, on any viewport.
 */
export function TransitionFailureNotice() {
  const { t } = useTranslation();
  const failure = useAppStore((state) => state.transitionFailureNotice);
  const dismiss = useAppStore((state) => state.setTransitionFailureNotice);
  if (!failure) return null;
  const headlineKey = transitionFailureHeadlineKey(failure.code);
  return (
    <div
      className="fixed inset-x-2 bottom-[max(0.5rem,env(safe-area-inset-bottom))] z-50 mx-auto max-w-md"
      data-testid="transition-failure-notice"
    >
      <Alert variant="destructive" role="alert" className="bg-background shadow-lg">
        <IconAlertTriangle />
        <AlertTitle className="flex items-center justify-between gap-2">
          {t("task:failedToMoveTask")}
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="h-6 w-6 shrink-0 cursor-pointer [@media(pointer:coarse)]:min-h-11 [@media(pointer:coarse)]:min-w-11"
            aria-label={t("common:close")}
            onClick={() => dismiss(null)}
            data-testid="transition-failure-notice-dismiss"
          >
            <IconX className="h-4 w-4" />
          </Button>
        </AlertTitle>
        <AlertDescription>
          {t(headlineKey ?? "task:taskMoveErrorGeneric")}
          <TransitionFailureDetails failure={failure} />
        </AlertDescription>
      </Alert>
    </div>
  );
}
