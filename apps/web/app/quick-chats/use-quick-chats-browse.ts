"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { listQuickChatsForBrowse } from "@/lib/api/domains/workspace-api";
import {
  buildQuickChatBrowseItems,
  sortQuickChatBrowseItems,
  type QuickChatBrowseItem,
  type QuickChatBrowseOrder,
} from "@/lib/quick-chat/browse";
import { persistQuickChatRename } from "@/lib/quick-chat/rename";

async function deleteQuickChatTask(taskId: string): Promise<void> {
  const { deleteTask } = await import("@/lib/api/domains/kanban-api");
  await deleteTask(taskId);
}

/** Data and actions for the Quick Chats browse page. */
export function useQuickChatsBrowse() {
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const openQuickChat = useAppStore((state) => state.openQuickChat);
  const removeQuickChatSession = useAppStore((state) => state.removeQuickChatSession);
  const renameQuickChatSession = useAppStore((state) => state.renameQuickChatSession);
  const [items, setItems] = useState<QuickChatBrowseItem[]>([]);
  const [status, setStatus] = useState<"loading" | "ready" | "error">("loading");
  const [order, setOrder] = useState<QuickChatBrowseOrder>("recent");
  const sortedItems = useMemo(() => sortQuickChatBrowseItems(items, order), [items, order]);
  const [sessionToDelete, setSessionToDelete] = useState<string | null>(null);
  const [sessionToRename, setSessionToRename] = useState<QuickChatBrowseItem | null>(null);

  const load = useCallback(async () => {
    if (!workspaceId) return;
    setStatus("loading");
    try {
      setItems(buildQuickChatBrowseItems((await listQuickChatsForBrowse(workspaceId)).sessions));
      setStatus("ready");
    } catch {
      setStatus("error");
    }
  }, [workspaceId]);

  useEffect(() => {
    void load();
  }, [load]);

  const deleteTarget = useMemo(
    () => items.find((item) => item.sessionId === sessionToDelete) ?? null,
    [items, sessionToDelete],
  );

  const handleConfirmDelete = useCallback(async () => {
    if (!deleteTarget) return;
    const target = deleteTarget;
    setSessionToDelete(null);
    try {
      await deleteQuickChatTask(target.taskId);
      removeQuickChatSession(target.sessionId);
      setItems((current) => current.filter((item) => item.sessionId !== target.sessionId));
    } catch {
      // Keep the row; a failed delete must not look successful.
    }
  }, [deleteTarget, removeQuickChatSession]);

  const handleRename = useCallback(
    async (name: string) => {
      const target = sessionToRename;
      setSessionToRename(null);
      if (!target) return;
      renameQuickChatSession(target.sessionId, name);
      setItems((current) =>
        current.map((item) => (item.sessionId === target.sessionId ? { ...item, name } : item)),
      );
      try {
        await persistQuickChatRename(target.sessionId, target.taskId, name);
      } catch {
        // The local rename stands; the next resync reconciles the server value.
      }
    },
    [renameQuickChatSession, sessionToRename],
  );

  const handleOpenInPanel = useCallback(
    (item: QuickChatBrowseItem) => {
      if (!workspaceId) return;
      openQuickChat(item.sessionId, workspaceId, undefined, item.kind, item.taskId);
    },
    [openQuickChat, workspaceId],
  );

  return {
    items: sortedItems,
    order,
    setOrder,
    status,
    reload: load,
    sessionToDelete,
    setSessionToDelete,
    sessionToRename,
    setSessionToRename,
    handleConfirmDelete,
    handleRename,
    handleOpenInPanel,
  };
}
