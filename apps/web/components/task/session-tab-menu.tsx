"use client";

import { useEffect, useState, type RefObject } from "react";
import {
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuSub,
  ContextMenuSubContent,
  ContextMenuSubTrigger,
} from "@kandev/ui/context-menu";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@kandev/ui/alert-dialog";
import { ActionConfirmPopover } from "@/components/confirmation/action-confirm-popover";
import {
  isSessionStoppable as isStoppable,
  isSessionDeletable as isDeletable,
  isSessionResumable as isResumable,
} from "@/hooks/domains/session/use-session-actions";
import { ShareDialog } from "@/components/task/share/share-dialog";
import { HandoffContextMenuSub } from "@/components/task/handoff-profile-menu-items";
import { NewSessionDialog, type HandoffPreset } from "@/components/task/new-session-dialog";
import type { TaskSessionState } from "@/lib/types/http";
import { useTranslation } from "react-i18next";
import { SessionDeleteDescription } from "./session-delete-description";
import {
  getTaskTransitionSummary,
  reassignTaskSessionStepVisit,
  type StepVisit,
} from "@/lib/api/domains/task-activity-api";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";

/** Lifecycle callbacks the context menu needs from the owning tab.
 * `handleClose`, `handleCloseOthers`, and `handleCloseToRight` are
 * dockview-specific (closing sibling panels) and have no equivalent where tabs
 * are a plain session switcher, so they're optional — omitting them hides those
 * items entirely. None of them deletes the underlying session. */
export type SessionTabMenuActions = {
  handleSetPrimary: () => void;
  handleStop: () => void;
  handleResume: () => void;
  handleClose?: () => void;
  handleCloseOthers?: () => void;
  handleCloseToRight?: () => void;
};

export function DeleteSessionDialog({
  open,
  onOpenChange,
  isPrimary,
  sessionCount,
  onConfirm,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  isPrimary: boolean;
  sessionCount: number;
  onConfirm: () => void;
}) {
  const { t } = useTranslation();
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t("task:deleteSession")}</AlertDialogTitle>
          <AlertDialogDescription asChild>
            <div className="min-w-0 space-y-2 text-left">
              <SessionDeleteDescription
                isPrimary={isPrimary}
                isOnlySession={sessionCount === 1}
                structured
              />
            </div>
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel className="cursor-pointer">{t("common:cancel")}</AlertDialogCancel>
          <AlertDialogAction
            onClick={() => {
              onOpenChange(false);
              onConfirm();
            }}
            className="cursor-pointer bg-destructive text-destructive-foreground hover:bg-destructive/90"
          >
            {t("task:delete")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

export function DeleteSessionPopover({
  open,
  anchorRef,
  focusBoundaryRef,
  onOpenChange,
  isPrimary,
  sessionCount,
  targetName,
  onConfirm,
}: {
  open: boolean;
  anchorRef: RefObject<HTMLElement | null>;
  focusBoundaryRef?: RefObject<HTMLElement | null>;
  onOpenChange: (open: boolean) => void;
  isPrimary: boolean;
  sessionCount: number;
  targetName?: string | null;
  onConfirm: () => void | Promise<void>;
}) {
  const { t } = useTranslation();
  const name = targetName || t("task:session");
  return (
    <ActionConfirmPopover
      open={open}
      anchorRef={anchorRef}
      focusBoundaryRef={focusBoundaryRef}
      title={t("task:deleteSession")}
      description={
        <SessionDeleteDescription isPrimary={isPrimary} isOnlySession={sessionCount === 1} />
      }
      cancelLabel={t("common:cancel")}
      confirmLabel={t("task:delete")}
      confirmAriaLabel={t("task:delete2", { name })}
      confirmTestId="session-delete-confirm"
      testId="session-delete-confirm-popover"
      onOpenChange={onOpenChange}
      onConfirm={onConfirm}
    />
  );
}

function SessionCloseMenuItems({ actions }: { actions: SessionTabMenuActions }) {
  const { t } = useTranslation();
  const { handleClose, handleCloseOthers, handleCloseToRight } = actions;
  if (!handleClose) return null;
  return (
    <>
      <ContextMenuItem className="cursor-pointer" onSelect={handleClose}>
        {t("task:close")}
      </ContextMenuItem>
      {handleCloseOthers && (
        <ContextMenuItem className="cursor-pointer" onSelect={handleCloseOthers}>
          {t("task:closeOthers")}
        </ContextMenuItem>
      )}
      {handleCloseToRight && (
        <ContextMenuItem className="cursor-pointer" onSelect={handleCloseToRight}>
          {t("task:closeTabsToRight")}
        </ContextMenuItem>
      )}
      <ContextMenuSeparator />
    </>
  );
}

export function SessionContextMenuItems({
  sessionState,
  isPrimary,
  canShare,
  taskId,
  sessionId,
  associatedStepVisitID,
  actions,
  onDelete,
  onShare,
  onHandoffProfile,
  onStartRename,
}: {
  sessionState: TaskSessionState | null;
  isPrimary: boolean;
  canShare: boolean;
  taskId: string | null;
  sessionId: string | undefined;
  associatedStepVisitID?: string;
  actions: SessionTabMenuActions;
  /** Passes the Radix selection event so the owner can capture its currentTarget as an anchor. */
  onDelete: (event: Event) => void;
  onShare: () => void;
  onHandoffProfile: (profileId: string) => void;
  onStartRename: () => void;
}) {
  const { t } = useTranslation();
  return (
    <ContextMenuContent>
      <SessionCloseMenuItems actions={actions} />
      <ContextMenuItem className="cursor-pointer" onSelect={onStartRename}>
        {t("task:rename2")}
      </ContextMenuItem>
      <ContextMenuItem
        className="cursor-pointer"
        onSelect={actions.handleSetPrimary}
        disabled={isPrimary || !sessionState || !isStoppable(sessionState)}
      >
        {t("task:setAsPrimary")}
      </ContextMenuItem>
      <ContextMenuSeparator />
      {taskId && sessionId && (
        <SessionStepVisitMenu
          taskId={taskId}
          sessionId={sessionId}
          associatedStepVisitID={associatedStepVisitID}
        />
      )}
      {sessionState && isStoppable(sessionState) && (
        <ContextMenuItem className="cursor-pointer" onSelect={actions.handleStop}>
          {t("task:stop")}
        </ContextMenuItem>
      )}
      {sessionState && isResumable(sessionState) && (
        <ContextMenuItem className="cursor-pointer" onSelect={actions.handleResume}>
          {t("task:resume")}
        </ContextMenuItem>
      )}
      {sessionState && isDeletable(sessionState) && (
        <ContextMenuItem
          className="cursor-pointer text-destructive"
          onSelect={(event) => {
            // Keep the context menu mounted so the confirmation popover can stay anchored here.
            event.preventDefault();
            onDelete(event);
          }}
        >
          {t("task:delete")}
        </ContextMenuItem>
      )}
      {canShare && (
        <>
          <ContextMenuSeparator />
          <ContextMenuItem className="cursor-pointer" onSelect={onShare}>
            {t("task:share")}
          </ContextMenuItem>
        </>
      )}
      {taskId && sessionId && (
        <>
          <ContextMenuSeparator />
          <HandoffContextMenuSub taskId={taskId} onSelectProfile={onHandoffProfile} />
        </>
      )}
    </ContextMenuContent>
  );
}

function SessionStepVisitMenu({
  taskId,
  sessionId,
  associatedStepVisitID,
}: {
  taskId: string;
  sessionId: string;
  associatedStepVisitID?: string;
}) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const session = useAppStore((state) => state.taskSessions.items[sessionId]);
  const setTaskSession = useAppStore((state) => state.setTaskSession);
  const [open, setOpen] = useState(false);
  const [visits, setVisits] = useState<StepVisit[]>([]);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!open) return;
    let cancelled = false;
    setLoading(true);
    getTaskTransitionSummary(taskId)
      .then((summary) => {
        if (!cancelled) setVisits(summary.visits);
      })
      .catch(() => {
        if (!cancelled) setVisits([]);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [open, taskId]);

  const assign = async (visitID: string) => {
    try {
      await reassignTaskSessionStepVisit(taskId, sessionId, visitID, associatedStepVisitID ?? "");
      if (session) setTaskSession({ ...session, associated_step_visit_id: visitID || undefined });
    } catch {
      toast({ title: t("task:sessionVisitUpdateFailed"), variant: "error" });
    }
  };

  const eligibleVisits = visits.filter((visit) => visit.visit_id && visit.eligible);
  return (
    <ContextMenuSub open={open} onOpenChange={setOpen}>
      <ContextMenuSubTrigger className="[@media(pointer:coarse)]:min-h-11">
        {t("task:associateWorkflowStep")}
      </ContextMenuSubTrigger>
      <ContextMenuSubContent className="max-h-72 w-64 overflow-y-auto">
        <ContextMenuItem
          className="[@media(pointer:coarse)]:min-h-11"
          onSelect={() => void assign("")}
        >
          <span className="flex-1">{t("task:workflowVisitUnassigned")}</span>
          {!associatedStepVisitID && <span>{t("kanban:current")}</span>}
        </ContextMenuItem>
        {loading ? (
          <ContextMenuItem disabled>{t("common:loading")}</ContextMenuItem>
        ) : (
          eligibleVisits.map((visit) => (
            <ContextMenuItem
              key={visit.visit_id}
              className="[@media(pointer:coarse)]:min-h-11"
              onSelect={() => void assign(visit.visit_id!)}
            >
              <span className="flex-1 truncate">{visit.step_name ?? visit.step_id}</span>
              <span className="text-xs text-muted-foreground">
                {t("task:workflowVisitOrdinal", { number: visit.ordinal ?? 0 })}
              </span>
              {visit.visit_id === associatedStepVisitID && <span>{t("kanban:current")}</span>}
            </ContextMenuItem>
          ))
        )}
      </ContextMenuSubContent>
    </ContextMenuSub>
  );
}

/** Delete / share / handoff dialogs rendered alongside the tab. */
export function SessionTabDialogs({
  confirmDelete,
  menuDeleteOpen,
  menuDeleteAnchorRef,
  menuDeleteFocusBoundaryRef,
  setConfirmDelete,
  isPrimary,
  sessionCount,
  onConfirmDelete,
  targetName,
  taskId,
  sessionId,
  shareOpen,
  setShareOpen,
  handoffOpen,
  setHandoffOpen,
  handoffPreset,
  setHandoffPreset,
  groupId,
}: {
  confirmDelete: boolean;
  menuDeleteOpen: boolean;
  menuDeleteAnchorRef: RefObject<HTMLElement | null>;
  menuDeleteFocusBoundaryRef?: RefObject<HTMLElement | null>;
  setConfirmDelete: (open: boolean) => void;
  isPrimary: boolean;
  sessionCount: number;
  onConfirmDelete: () => void;
  targetName?: string | null;
  taskId: string | null;
  sessionId: string | undefined;
  shareOpen: boolean;
  setShareOpen: (open: boolean) => void;
  handoffOpen: boolean;
  setHandoffOpen: (open: boolean) => void;
  handoffPreset: HandoffPreset | null;
  setHandoffPreset: (preset: HandoffPreset | null) => void;
  groupId: string | undefined;
}) {
  return (
    <>
      <DeleteSessionDialog
        open={confirmDelete && !menuDeleteOpen}
        onOpenChange={setConfirmDelete}
        isPrimary={isPrimary}
        sessionCount={sessionCount}
        onConfirm={onConfirmDelete}
      />
      <DeleteSessionPopover
        open={menuDeleteOpen}
        anchorRef={menuDeleteAnchorRef}
        focusBoundaryRef={menuDeleteFocusBoundaryRef}
        onOpenChange={setConfirmDelete}
        isPrimary={isPrimary}
        sessionCount={sessionCount}
        targetName={targetName}
        onConfirm={onConfirmDelete}
      />
      {taskId && sessionId && (
        <ShareDialog
          open={shareOpen}
          onOpenChange={setShareOpen}
          taskId={taskId}
          sessionId={sessionId}
        />
      )}
      {taskId && handoffPreset && (
        <NewSessionDialog
          open={handoffOpen}
          onOpenChange={(open) => {
            setHandoffOpen(open);
            if (!open) setHandoffPreset(null);
          }}
          taskId={taskId}
          groupId={groupId}
          handoff={handoffPreset}
        />
      )}
    </>
  );
}
