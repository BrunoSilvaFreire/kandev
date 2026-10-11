"use client";

import { useEffect, useRef, useState, type RefObject } from "react";
import { IconDotsVertical } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useAppStore } from "@/components/state-provider";
import {
  isSessionStoppable,
  isSessionDeletable,
  isSessionResumable,
} from "@/hooks/domains/session/use-session-actions";
import { HandoffDropdownMenuSub } from "../handoff-profile-menu-items";
import { useTranslation } from "react-i18next";
import {
  getTaskTransitionSummary,
  reassignTaskSessionStepVisit,
  type StepVisit,
} from "@/lib/api/domains/task-activity-api";
import { useToast } from "@/components/toast-provider";
import { MobilePickerSheet } from "./mobile-picker-sheet";
import type { TaskSessionState } from "@/lib/types/http";

function useStepVisits(taskId: string, open: boolean) {
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
  }, [taskId, open]);
  return { visits, loading };
}

function visitLabel(visit: StepVisit, t: (key: string, options?: { number: number }) => string) {
  const ordinal = t("task:workflowVisitOrdinal", { number: visit.ordinal ?? 0 });
  return `${visit.step_name ?? visit.step_id} ${ordinal}`;
}

function VisitAssignmentItem({
  taskId,
  sessionId,
  associatedStepVisitID,
  targetVisitId,
  label,
  selected,
}: {
  taskId: string;
  sessionId: string;
  associatedStepVisitID?: string;
  targetVisitId: string;
  label: string;
  selected: boolean;
}) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const session = useAppStore((state) => state.taskSessions.items[sessionId]);
  const setTaskSession = useAppStore((state) => state.setTaskSession);
  const assignVisit = async () => {
    try {
      await reassignTaskSessionStepVisit(
        taskId,
        sessionId,
        targetVisitId,
        associatedStepVisitID ?? "",
      );
      if (session)
        setTaskSession({ ...session, associated_step_visit_id: targetVisitId || undefined });
    } catch {
      toast({ title: t("task:sessionVisitUpdateFailed"), variant: "error" });
    }
  };
  return (
    <DropdownMenuItem
      className="[@media(pointer:coarse)]:min-h-11"
      onSelect={() => void assignVisit()}
    >
      <span className="flex-1 truncate">{label}</span>
      {selected && <span>{t("kanban:current")}</span>}
    </DropdownMenuItem>
  );
}

function VisitAssignmentButton({
  taskId,
  sessionId,
  associatedStepVisitID,
  targetVisitId,
  label,
  selected,
  onAssigned,
}: {
  taskId: string;
  sessionId: string;
  associatedStepVisitID?: string;
  targetVisitId: string;
  label: string;
  selected: boolean;
  onAssigned: () => void;
}) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const session = useAppStore((state) => state.taskSessions.items[sessionId]);
  const setTaskSession = useAppStore((state) => state.setTaskSession);
  const assignVisit = async () => {
    try {
      await reassignTaskSessionStepVisit(
        taskId,
        sessionId,
        targetVisitId,
        associatedStepVisitID ?? "",
      );
      if (session)
        setTaskSession({ ...session, associated_step_visit_id: targetVisitId || undefined });
      onAssigned();
    } catch {
      toast({ title: t("task:sessionVisitUpdateFailed"), variant: "error" });
    }
  };
  return (
    <Button
      type="button"
      variant="ghost"
      className="min-h-11 w-full justify-start"
      onClick={() => void assignVisit()}
    >
      <span className="flex-1 truncate text-left">{label}</span>
      {selected && <span>{t("kanban:current")}</span>}
    </Button>
  );
}

type VisitMenuProps = {
  taskId: string;
  sessionId: string;
  associatedStepVisitID?: string;
  visits: StepVisit[];
  loading: boolean;
};

function eligibleVisits(visits: StepVisit[]) {
  return visits.filter((visit) => visit.visit_id && visit.eligible);
}

function SessionStepVisitMenuItems({
  taskId,
  sessionId,
  associatedStepVisitID,
  visits,
  loading,
}: VisitMenuProps) {
  const { t } = useTranslation();
  return (
    <>
      <VisitAssignmentItem
        taskId={taskId}
        sessionId={sessionId}
        associatedStepVisitID={associatedStepVisitID}
        targetVisitId=""
        label={t("task:workflowVisitUnassigned")}
        selected={!associatedStepVisitID}
      />
      {loading ? (
        <DropdownMenuItem disabled>{t("common:loading")}</DropdownMenuItem>
      ) : (
        eligibleVisits(visits).map((visit) => (
          <VisitAssignmentItem
            key={visit.visit_id}
            taskId={taskId}
            sessionId={sessionId}
            associatedStepVisitID={associatedStepVisitID}
            targetVisitId={visit.visit_id!}
            label={visitLabel(visit, t)}
            selected={visit.visit_id === associatedStepVisitID}
          />
        ))
      )}
    </>
  );
}

function MobileSessionVisitSheet({
  open,
  onOpenChange,
  taskId,
  sessionId,
  associatedStepVisitID,
  visits,
  loading,
}: VisitMenuProps & { open: boolean; onOpenChange: (open: boolean) => void }) {
  const { t } = useTranslation();
  const close = () => onOpenChange(false);
  return (
    <MobilePickerSheet
      open={open}
      onOpenChange={onOpenChange}
      title={t("task:associateWorkflowStep")}
      contentTestId="mobile-session-step-visits"
    >
      <div className="space-y-1">
        <VisitAssignmentButton
          taskId={taskId}
          sessionId={sessionId}
          associatedStepVisitID={associatedStepVisitID}
          targetVisitId=""
          label={t("task:workflowVisitUnassigned")}
          selected={!associatedStepVisitID}
          onAssigned={close}
        />
        {loading ? (
          <Button disabled variant="ghost" className="min-h-11 w-full justify-start">
            {t("common:loading")}
          </Button>
        ) : (
          eligibleVisits(visits).map((visit) => (
            <VisitAssignmentButton
              key={visit.visit_id}
              taskId={taskId}
              sessionId={sessionId}
              associatedStepVisitID={associatedStepVisitID}
              targetVisitId={visit.visit_id!}
              label={visitLabel(visit, t)}
              selected={visit.visit_id === associatedStepVisitID}
              onAssigned={close}
            />
          ))
        )}
      </div>
    </MobilePickerSheet>
  );
}

function SessionActionsTrigger({
  triggerRef,
  label,
}: {
  triggerRef: RefObject<HTMLButtonElement | null>;
  label: string;
}) {
  return (
    <DropdownMenuTrigger asChild>
      <Button
        ref={triggerRef}
        variant="ghost"
        size="icon-sm"
        className="cursor-pointer h-11 w-11 md:h-7 md:w-7"
        onClick={(e) => e.stopPropagation()}
        aria-label={label}
      >
        <IconDotsVertical className="h-4 w-4" />
      </Button>
    </DropdownMenuTrigger>
  );
}

function SessionLifecycleMenuItems({
  state,
  onStop,
  onResume,
  onDelete,
}: {
  state: TaskSessionState | null;
  onStop: () => void;
  onResume: () => void;
  onDelete: () => void;
}) {
  const { t } = useTranslation();
  return (
    <>
      {state && isSessionStoppable(state) && (
        <DropdownMenuItem className="cursor-pointer" onSelect={onStop}>
          {t("task:stop")}
        </DropdownMenuItem>
      )}
      {state && isSessionResumable(state) && (
        <DropdownMenuItem className="cursor-pointer" onSelect={onResume}>
          {t("task:resume")}
        </DropdownMenuItem>
      )}
      {state && isSessionDeletable(state) && (
        <DropdownMenuItem
          className="cursor-pointer text-destructive focus:text-destructive"
          onSelect={onDelete}
        >
          {t("task:delete")}
        </DropdownMenuItem>
      )}
    </>
  );
}

export function SessionActionsMenu({
  triggerRef,
  taskId,
  sessionId,
  associatedStepVisitID,
  state,
  isPrimary,
  onSetPrimary,
  onStop,
  onResume,
  onAskDelete,
  onHandoffProfile,
}: {
  triggerRef: RefObject<HTMLButtonElement | null>;
  taskId: string;
  sessionId: string;
  associatedStepVisitID?: string;
  state: TaskSessionState | null;
  isPrimary: boolean;
  onSetPrimary: () => void;
  onStop: () => void;
  onResume: () => void;
  onAskDelete: () => void;
  onHandoffProfile: (profileId: string) => void;
}) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const pendingDelete = useRef(false);
  const [visitMenuOpen, setVisitMenuOpen] = useState(false);
  const { visits, loading: visitsLoading } = useStepVisits(taskId, visitMenuOpen);
  const hasLifecycleAction =
    !!state &&
    (isSessionStoppable(state) || isSessionResumable(state) || isSessionDeletable(state));
  const visitProps = { taskId, sessionId, associatedStepVisitID, visits, loading: visitsLoading };
  const requestDelete = () => {
    if (isMobile) pendingDelete.current = true;
    else onAskDelete();
  };

  return (
    <>
      <DropdownMenu>
        <SessionActionsTrigger triggerRef={triggerRef} label={t("task:sessionActions")} />
        <DropdownMenuContent
          align="end"
          onClick={(e) => e.stopPropagation()}
          onCloseAutoFocus={(event) => {
            if (!pendingDelete.current) return;
            event.preventDefault();
            pendingDelete.current = false;
            onAskDelete();
          }}
        >
          <DropdownMenuItem
            className="cursor-pointer"
            onSelect={onSetPrimary}
            disabled={isPrimary || !state}
          >
            {t("task:setAsPrimary")}
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          {isMobile ? (
            <DropdownMenuItem
              className="[@media(pointer:coarse)]:min-h-11"
              onSelect={() => setVisitMenuOpen(true)}
            >
              {t("task:associateWorkflowStep")}
            </DropdownMenuItem>
          ) : (
            <DropdownMenuSub open={visitMenuOpen} onOpenChange={setVisitMenuOpen}>
              <DropdownMenuSubTrigger className="[@media(pointer:coarse)]:min-h-11">
                {t("task:associateWorkflowStep")}
              </DropdownMenuSubTrigger>
              <DropdownMenuSubContent className="max-h-72 w-64 overflow-y-auto">
                <SessionStepVisitMenuItems {...visitProps} />
              </DropdownMenuSubContent>
            </DropdownMenuSub>
          )}
          <DropdownMenuSeparator />
          <SessionLifecycleMenuItems
            state={state}
            onStop={onStop}
            onResume={onResume}
            onDelete={requestDelete}
          />
          {hasLifecycleAction && <DropdownMenuSeparator />}
          <HandoffDropdownMenuSub taskId={taskId} onSelectProfile={onHandoffProfile} />
        </DropdownMenuContent>
      </DropdownMenu>
      {isMobile && (
        <MobileSessionVisitSheet
          open={visitMenuOpen}
          onOpenChange={setVisitMenuOpen}
          {...visitProps}
        />
      )}
    </>
  );
}
