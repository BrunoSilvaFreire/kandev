"use client";

import { useCallback, useState } from "react";
import { useTranslation } from "react-i18next";
import { useToast } from "@/components/toast-provider";
import { cancelPtyTerminalStart } from "@/components/settings/pty-terminal-lifecycle";
import { deleteQuickTerminalTab } from "@/lib/api/domains/quick-terminal-api";
import { ApiError } from "@/lib/api/client";
import { isQuickChatSetupSessionId } from "@/lib/state/slices/ui/quick-chat-session";
import type {
  QuickChatActiveKind,
  QuickChatSession,
  QuickTerminalTab,
  QuickTerminalUpdate,
} from "@/lib/state/slices/ui/types";
import {
  adjacentQuickChatTabReference,
  conversationTabReference,
  terminalTabReference,
} from "./use-quick-chat-tab-order";

export type QuickChatCloseStore = {
  sessions: QuickChatSession[];
  terminalTabs: QuickTerminalTab[];
  activeSessionId: string | null;
  activeKind: QuickChatActiveKind;
  activeTerminalTabId: string | null;
  taskSessions: Record<string, { task_id: string }>;
  closeQuickChatSession: (sessionId: string) => void;
  removeQuickChatSession: (sessionId: string) => void;
  setActiveQuickChatSession: (sessionId: string, workspaceId: string) => void;
  activateQuickTerminal: (tabId: string, workspaceId: string) => void;
  updateQuickTerminal: (tabId: string, update: QuickTerminalUpdate) => void;
  removeQuickTerminal: (tabId: string) => void;
};

export function resolveQuickChatTaskId(
  store: Pick<QuickChatCloseStore, "sessions" | "taskSessions">,
  sessionId: string,
): string | undefined {
  return (
    store.sessions.find((session) => session.sessionId === sessionId)?.taskId ??
    store.taskSessions[sessionId]?.task_id
  );
}

async function deleteQuickChatTask(taskId: string) {
  const { deleteTaskAfterUserAction } = await import("@/lib/api/domains/kanban-api");
  await deleteTaskAfterUserAction(taskId);
}

type QuickChatTabCloseNavigation = {
  tabOrder: string[];
  activeTabReference: string | undefined;
  onActivateTabReference: (reference: string) => void;
};

function getActiveQuickChatTabReference(
  store: Pick<QuickChatCloseStore, "activeKind" | "activeSessionId" | "activeTerminalTabId">,
): string | undefined {
  if (store.activeKind === "conversation" && store.activeSessionId) {
    return conversationTabReference(store.activeSessionId);
  }
  if (store.activeKind === "terminal" && store.activeTerminalTabId) {
    return terminalTabReference(store.activeTerminalTabId);
  }
  return undefined;
}

function activateQuickChatTabReference(
  store: Pick<QuickChatCloseStore, "setActiveQuickChatSession" | "activateQuickTerminal">,
  workspaceId: string,
  reference: string,
): void {
  if (reference.startsWith("conversation:")) {
    const sessionId = reference.slice("conversation:".length);
    if (sessionId) store.setActiveQuickChatSession(sessionId, workspaceId);
    return;
  }
  if (reference.startsWith("terminal:")) {
    const tabId = reference.slice("terminal:".length);
    if (tabId) store.activateQuickTerminal(tabId, workspaceId);
  }
}

function useQuickChatSessionClose(
  store: QuickChatCloseStore,
  resetPendingStarts: () => void,
  removeTabReference: (reference: string) => void,
  navigation: QuickChatTabCloseNavigation,
) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const { activeTabReference, onActivateTabReference, tabOrder } = navigation;
  const [sessionToClose, setSessionToClose] = useState<string | null>(null);
  const [replacementReference, setReplacementReference] = useState<string | null>(null);

  // Closing a tab never touches the server. It removes the chat from the
  // persisted tab order and tombstones the local session so reconciliation
  // does not re-add it. Deletion is a separate, confirmed action.
  const closeTab = useCallback(
    (sessionId: string) => {
      resetPendingStarts();
      const reference = conversationTabReference(sessionId);
      const replacement =
        activeTabReference === reference
          ? (adjacentQuickChatTabReference(tabOrder, reference) ?? null)
          : null;
      if (isQuickChatSetupSessionId(sessionId)) {
        store.closeQuickChatSession(sessionId);
      } else {
        store.removeQuickChatSession(sessionId);
      }
      removeTabReference(reference);
      if (replacement) onActivateTabReference(replacement);
    },
    [
      activeTabReference,
      onActivateTabReference,
      removeTabReference,
      resetPendingStarts,
      store,
      tabOrder,
    ],
  );

  const handleCloseTab = useCallback((sessionId: string) => closeTab(sessionId), [closeTab]);

  // Context-menu "Delete conversation": keep the tab until the user confirms.
  const handleRequestDelete = useCallback(
    (sessionId: string) => {
      resetPendingStarts();
      if (isQuickChatSetupSessionId(sessionId)) return;
      const reference = conversationTabReference(sessionId);
      setReplacementReference(
        activeTabReference === reference
          ? (adjacentQuickChatTabReference(tabOrder, reference) ?? null)
          : null,
      );
      setSessionToClose(sessionId);
    },
    [activeTabReference, resetPendingStarts, tabOrder],
  );

  const handleConfirmClose = useCallback(async () => {
    if (!sessionToClose) return;
    const sessionId = sessionToClose;
    const replacement = replacementReference;
    setSessionToClose(null);
    setReplacementReference(null);
    const taskId = resolveQuickChatTaskId(store, sessionId);
    if (!taskId) {
      store.removeQuickChatSession(sessionId);
      removeTabReference(conversationTabReference(sessionId));
      if (replacement) onActivateTabReference(replacement);
      return;
    }
    try {
      await deleteQuickChatTask(taskId);
      store.removeQuickChatSession(sessionId);
      removeTabReference(conversationTabReference(sessionId));
      if (replacement) onActivateTabReference(replacement);
    } catch (error) {
      console.error("Failed to delete quick chat task:", error);
      toast({
        title: t("chat:failedToDeleteQuickChat"),
        description: error instanceof Error ? error.message : t("chat:unknownError"),
        variant: "error",
      });
    }
  }, [
    onActivateTabReference,
    removeTabReference,
    replacementReference,
    sessionToClose,
    store,
    t,
    toast,
  ]);

  return {
    sessionToClose,
    setSessionToClose,
    handleCloseTab,
    handleRequestDelete,
    handleConfirmClose,
  };
}

function useQuickTerminalClose(
  store: QuickChatCloseStore,
  resetPendingStarts: () => void,
  removeTabReference: (reference: string) => void,
  navigation: QuickChatTabCloseNavigation,
) {
  const { toast } = useToast();
  const { t } = useTranslation();
  const { activeTabReference, onActivateTabReference, tabOrder } = navigation;

  const handleCloseTerminal = useCallback(
    async (tabId: string) => {
      resetPendingStarts();
      cancelPtyTerminalStart(tabId);
      const tab = store.terminalTabs.find((item) => item.tabId === tabId);
      if (!tab) return;
      const reference = terminalTabReference(tabId);
      const replacement =
        activeTabReference === reference
          ? adjacentQuickChatTabReference(tabOrder, reference)
          : undefined;
      try {
        await deleteQuickTerminalTab(tabId);
      } catch (error) {
        if (error instanceof ApiError && error.status === 404) {
          store.removeQuickTerminal(tabId);
          removeTabReference(reference);
          if (replacement) onActivateTabReference(replacement);
          return;
        }
        const message = error instanceof Error ? error.message : String(error);
        store.updateQuickTerminal(tabId, { status: "error", error: message });
        toast({
          title: t("sidebar:quickChatTerminals"),
          description: t("sidebar:quickChatTerminalError", { error: message }),
          variant: "error",
        });
        return;
      }
      store.removeQuickTerminal(tabId);
      removeTabReference(reference);
      if (replacement) onActivateTabReference(replacement);
    },
    [
      activeTabReference,
      onActivateTabReference,
      removeTabReference,
      resetPendingStarts,
      store,
      t,
      tabOrder,
      toast,
    ],
  );

  return handleCloseTerminal;
}

type QuickChatCloseActionsOptions = {
  workspaceId: string;
  store: QuickChatCloseStore;
  resetPendingStarts: () => void;
  removeTabReference: (reference: string) => void;
  tabOrder: string[];
};

export function useQuickChatCloseActions({
  workspaceId,
  store,
  resetPendingStarts,
  removeTabReference,
  tabOrder,
}: QuickChatCloseActionsOptions) {
  const activeTabReference = getActiveQuickChatTabReference(store);
  const onActivateTabReference = useCallback(
    (reference: string) => activateQuickChatTabReference(store, workspaceId, reference),
    [store, workspaceId],
  );
  const navigation = { tabOrder, activeTabReference, onActivateTabReference };
  const sessionClose = useQuickChatSessionClose(
    store,
    resetPendingStarts,
    removeTabReference,
    navigation,
  );
  const handleCloseTerminal = useQuickTerminalClose(
    store,
    resetPendingStarts,
    removeTabReference,
    navigation,
  );

  const closeConversationReference = useCallback(
    (reference: string) => {
      const sessionId = reference.slice("conversation:".length);
      if (!sessionId) return;
      if (isQuickChatSetupSessionId(sessionId)) store.closeQuickChatSession(sessionId);
      else store.removeQuickChatSession(sessionId);
      removeTabReference(reference);
    },
    [removeTabReference, store],
  );

  // Close Others / Close Tabs to the Right operate on conversation tabs; the
  // target tab stays and becomes active when the previous active tab closed.
  const closeTargetReferences = useCallback(
    (sessionId: string, mode: "others" | "right") => {
      resetPendingStarts();
      const reference = conversationTabReference(sessionId);
      const index = tabOrder.indexOf(reference);
      if (index < 0) return;
      const targets = (
        mode === "others"
          ? tabOrder.filter((item) => item !== reference)
          : tabOrder.slice(index + 1)
      ).filter((item) => item.startsWith("conversation:"));
      const closed = new Set([reference, ...targets]);
      for (const target of targets) closeConversationReference(target);
      if (activeTabReference && closed.has(activeTabReference)) {
        onActivateTabReference(reference);
      }
    },
    [
      activeTabReference,
      closeConversationReference,
      onActivateTabReference,
      resetPendingStarts,
      tabOrder,
    ],
  );

  const handleCloseOthers = useCallback(
    (sessionId: string) => closeTargetReferences(sessionId, "others"),
    [closeTargetReferences],
  );
  const handleCloseToRight = useCallback(
    (sessionId: string) => closeTargetReferences(sessionId, "right"),
    [closeTargetReferences],
  );

  return { ...sessionClose, handleCloseTerminal, handleCloseOthers, handleCloseToRight };
}
