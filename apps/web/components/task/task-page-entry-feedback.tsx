"use client";

import { EnsureSessionErrorBanner } from "@/components/task/ensure-session-error";
import {
  TaskMoveErrorBanner,
  TaskTransitionFailureBanner,
} from "@/components/task/task-move-error-banner";
import type { UseEnsureTaskSessionResult } from "@/hooks/domains/session/use-ensure-task-session";
import type { TransitionFailureRecord } from "@/lib/tasks/transition-failure";

/** Page-level banners shown above the task layout on entry and after a move. */
export function TaskPageEntryFeedback({
  taskMoveError,
  transitionFailure,
  ensureSession,
  workspaceId,
}: {
  taskMoveError: unknown;
  transitionFailure: TransitionFailureRecord | null;
  ensureSession: UseEnsureTaskSessionResult;
  workspaceId: string | null;
}) {
  return (
    <>
      {taskMoveError !== null && <TaskMoveErrorBanner error={taskMoveError} />}
      {transitionFailure && <TaskTransitionFailureBanner record={transitionFailure} />}
      {ensureSession.status === "error" && (
        <EnsureSessionErrorBanner
          error={ensureSession.error}
          onRetry={ensureSession.retry}
          workspaceId={workspaceId}
        />
      )}
    </>
  );
}
