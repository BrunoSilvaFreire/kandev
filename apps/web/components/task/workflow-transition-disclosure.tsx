"use client";

import { useCallback, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconChevronDown, IconChevronUp } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Drawer, DrawerContent, DrawerHeader, DrawerTitle, DrawerTrigger } from "@kandev/ui/drawer";
import { useAppStore } from "@/components/state-provider";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { useTaskTransitionSummary } from "@/hooks/domains/task/use-task-activity";
import { useActivateTaskSession } from "./use-activate-task-session";
import { WorkflowTransitionSummary } from "./workflow-transition-summary";
import type { WorkflowStepperStep } from "./workflow-step-disclosure";

type WorkflowTransitionDisclosureProps = {
  steps: WorkflowStepperStep[];
  currentStepId: string | null;
  taskId: string | null;
};

/**
 * The task header's small expand/collapse control. Compact mode is the
 * default; expansion is local UI state and is never persisted. Fine-pointer
 * desktop renders a 28px control with a bounded popover; coarse pointers get a
 * >=44px control opening the existing touch drawer. The connector surface is
 * the semantic transition summary, so the header never overflows.
 */
export function WorkflowTransitionDisclosure({
  steps,
  currentStepId,
  taskId,
}: WorkflowTransitionDisclosureProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const useDrawer = useTouchDrawer();
  const summary = useTaskTransitionSummary(taskId, Boolean(taskId));
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const setMobileTaskHistoryStepId = useAppStore((state) => state.setMobileTaskHistoryStepId);
  const setMobileSessionPanel = useAppStore((state) => state.setMobileSessionPanel);
  const openTaskHistory = useDockviewStore((state) => state.openTaskHistory);
  const activateSession = useActivateTaskSession();

  const handleOpenSession = useCallback(
    (sessionId: string) => {
      setOpen(false);
      activateSession(sessionId);
    },
    [activateSession],
  );

  const handleOpenHistory = useCallback(
    (stepId: string) => {
      setOpen(false);
      if (useDrawer) {
        if (taskId) setMobileTaskHistoryStepId(taskId, stepId);
        if (activeSessionId) setMobileSessionPanel(activeSessionId, "task-history");
        return;
      }
      openTaskHistory(stepId);
    },
    [
      activeSessionId,
      openTaskHistory,
      setMobileSessionPanel,
      setMobileTaskHistoryStepId,
      taskId,
      useDrawer,
    ],
  );

  if (steps.length === 0) return null;

  const label = open ? t("task:workflowCollapse") : t("task:workflowExpand");
  const trigger = (
    <Button
      type="button"
      variant="ghost"
      size="sm"
      aria-label={label}
      title={label}
      data-testid="workflow-stepper-expand"
      className="ml-1 h-7 min-w-7 shrink-0 cursor-pointer px-1.5 [@media(pointer:coarse)]:h-11 [@media(pointer:coarse)]:min-w-11"
    >
      {open ? (
        <IconChevronUp className="h-3.5 w-3.5" aria-hidden="true" />
      ) : (
        <IconChevronDown className="h-3.5 w-3.5" aria-hidden="true" />
      )}
    </Button>
  );

  const content = (
    <WorkflowTransitionSummary
      steps={steps}
      currentStepId={currentStepId}
      summary={summary}
      onOpenSession={handleOpenSession}
      onOpenHistory={handleOpenHistory}
    />
  );

  if (useDrawer) {
    return (
      <Drawer open={open} onOpenChange={setOpen}>
        <DrawerTrigger asChild>{trigger}</DrawerTrigger>
        <DrawerContent data-testid="workflow-transition-drawer">
          <DrawerHeader>
            <DrawerTitle>{t("task:workflowTransitionsTitle")}</DrawerTitle>
          </DrawerHeader>
          <div
            className="overflow-y-auto px-4 pb-6"
            style={{
              maxHeight: "70dvh",
              paddingBottom: "calc(1.5rem + env(safe-area-inset-bottom, 0px))",
            }}
          >
            {content}
          </div>
        </DrawerContent>
      </Drawer>
    );
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>{trigger}</PopoverTrigger>
      <PopoverContent align="end" className="w-80" data-testid="workflow-transition-popover">
        {content}
      </PopoverContent>
    </Popover>
  );
}
