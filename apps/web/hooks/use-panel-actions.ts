"use client";

import { useCallback } from "react";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { useLayoutStore } from "@/lib/state/layout-store";
import { useAppStore } from "@/components/state-provider";
import { useFileEditors } from "@/hooks/use-file-editors";
import { useRouter } from "@/lib/routing/client-router";
import { linkToTask } from "@/lib/links";

/** Mobile/tablet plan fallback: open the session's plan document. */
function openPlanOnMobile(args: {
  sessionId: string;
  taskId: string;
  setActiveDocument: (sessionId: string, doc: { type: "plan"; taskId: string }) => void;
  openDocument: (sessionId: string) => void;
  setPlanMode: (sessionId: string, enabled: boolean) => void;
}): void {
  args.setActiveDocument(args.sessionId, { type: "plan", taskId: args.taskId });
  args.openDocument(args.sessionId);
  args.setPlanMode(args.sessionId, true);
}

/** Desktop plan path when the surface has no dockview (the Quick Chat panel). */
function navigateToPlanSurface(args: {
  taskId: string;
  isQuickChatTask: boolean;
  push: (href: string) => void;
}): void {
  args.push(
    args.isQuickChatTask
      ? `/quick-chats/${args.taskId}?panel=plan`
      : `${linkToTask(args.taskId)}?panel=plan`,
  );
}

function buildAddBrowserAction(args: {
  usesDesktopWorkbench: boolean;
  dockAddBrowser: (url?: string) => void;
  activeSessionId: string | null;
}): (url?: string) => void {
  return (url?: string) => {
    if (args.usesDesktopWorkbench) {
      args.dockAddBrowser(url);
    } else if (args.activeSessionId) {
      // Mobile/tablet: use layout store to open preview
      useLayoutStore.getState().openPreview(args.activeSessionId);
    }
  };
}

type AddPlanArgs = {
  usesDesktopWorkbench: boolean;
  hasDockview: boolean;
  dockAddPlan: () => void;
  activeSessionId: string | null;
  activeTaskId: string | null;
  setActiveDocument: (sessionId: string, doc: { type: "plan"; taskId: string }) => void;
  openDocument: (sessionId: string) => void;
  setPlanMode: (sessionId: string, enabled: boolean) => void;
  isQuickChatTask: boolean;
  push: (href: string) => void;
};

function buildAddPlanAction(args: AddPlanArgs): () => void {
  return () => {
    if (args.usesDesktopWorkbench && args.hasDockview) {
      args.dockAddPlan();
      return;
    }
    if (!args.usesDesktopWorkbench && args.activeSessionId && args.activeTaskId) {
      openPlanOnMobile({
        sessionId: args.activeSessionId,
        taskId: args.activeTaskId,
        setActiveDocument: args.setActiveDocument,
        openDocument: args.openDocument,
        setPlanMode: args.setPlanMode,
      });
      return;
    }
    // Desktop without a dockview (the Quick Chat panel): route to the surface
    // that owns one so the plan CTA is never inert.
    if (args.usesDesktopWorkbench && args.activeTaskId) {
      navigateToPlanSurface({
        taskId: args.activeTaskId,
        isQuickChatTask: args.isQuickChatTask,
        push: args.push,
      });
    }
  };
}

/**
 * Unified hook returning add-only panel action functions.
 * On desktop: delegates to dockview store.
 * On mobile/tablet: delegates to layout store (kept for backward compat).
 */
export function usePanelActions() {
  const { usesDesktopWorkbench } = useResponsiveBreakpoint();

  // Desktop: dockview store
  const dockAddBrowser = useDockviewStore((s) => s.addBrowserPanel);
  const dockAddPlan = useDockviewStore((s) => s.addPlanPanel);
  const dockAddChat = useDockviewStore((s) => s.addChatPanel);
  const dockAddChanges = useDockviewStore((s) => s.addChangesPanel);
  const dockAddTerminal = useDockviewStore((s) => s.addTerminalPanel);
  const dockAddVscode = useDockviewStore((s) => s.addVscodePanel);

  // File editors (works on desktop through dockview)
  const { openFile: dockOpenFile, openFileInMarkdownPreview: dockOpenFileInPreview } =
    useFileEditors();

  // Mobile/Tablet: layout store
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const openDocument = useLayoutStore((s) => s.openDocument);
  const setActiveDocument = useAppStore((s) => s.setActiveDocument);
  const activeTaskId = useAppStore((s) => s.tasks.activeTaskId);
  const setPlanMode = useAppStore((s) => s.setPlanMode);
  const dockviewApi = useDockviewStore((s) => s.api);
  const isQuickChatTask = useAppStore((s) =>
    activeTaskId ? s.quickChat.sessions.some((session) => session.taskId === activeTaskId) : false,
  );
  const router = useRouter();

  const addBrowser = useCallback(
    buildAddBrowserAction({ usesDesktopWorkbench, dockAddBrowser, activeSessionId }),
    [usesDesktopWorkbench, dockAddBrowser, activeSessionId],
  );

  const addPlan = useCallback(
    buildAddPlanAction({
      usesDesktopWorkbench,
      hasDockview: Boolean(dockviewApi),
      dockAddPlan,
      activeSessionId,
      activeTaskId,
      setActiveDocument,
      openDocument,
      setPlanMode,
      isQuickChatTask,
      push: (href) => router.push(href),
    }),
    [
      usesDesktopWorkbench,
      dockviewApi,
      dockAddPlan,
      activeSessionId,
      activeTaskId,
      setActiveDocument,
      openDocument,
      setPlanMode,
      isQuickChatTask,
      router,
    ],
  );

  const addChat = useCallback(() => {
    if (usesDesktopWorkbench) {
      dockAddChat();
    }
  }, [usesDesktopWorkbench, dockAddChat]);

  const addChanges = useCallback(() => {
    if (usesDesktopWorkbench) {
      dockAddChanges();
    }
  }, [usesDesktopWorkbench, dockAddChanges]);

  const addTerminal = useCallback(
    (terminalId?: string) => {
      if (usesDesktopWorkbench) {
        dockAddTerminal(terminalId);
      }
    },
    [usesDesktopWorkbench, dockAddTerminal],
  );

  const addVscode = useCallback(() => {
    if (usesDesktopWorkbench) {
      dockAddVscode();
    }
  }, [usesDesktopWorkbench, dockAddVscode]);

  const openFile = useCallback(
    (filePath: string, repo?: string) => {
      if (usesDesktopWorkbench) {
        dockOpenFile(filePath, repo);
      }
    },
    [usesDesktopWorkbench, dockOpenFile],
  );

  const openFileInMarkdownPreview = useCallback(
    (filePath: string, repo?: string) => {
      if (usesDesktopWorkbench) {
        dockOpenFileInPreview(filePath, repo);
      }
    },
    [usesDesktopWorkbench, dockOpenFileInPreview],
  );

  return {
    addBrowser,
    addPlan,
    addChat,
    addChanges,
    addTerminal,
    addVscode,
    openFile,
    openFileInMarkdownPreview,
  };
}
