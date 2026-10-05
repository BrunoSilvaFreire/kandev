"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { listQuickChatSessions } from "@/lib/api/domains/workspace-api";
import { persistQuickChatRename } from "@/lib/quick-chat/rename";
import type { TaskSession } from "@/lib/types/http";
import type { QuickChatBrowseItem } from "./quick-chat-browse-row";

async function deleteQuickChatTask(taskId: string): Promise<void> {
  const { deleteTask } = await import("@/lib/api/domains/kanban-api");
  await deleteTask(taskId);
}

function buildBrowseItems(
  response: Awaited<ReturnType<typeof listQuickChatSessions>>,
): QuickChatBrowseItem[] {
  const updatedAtBySession = new Map<string, string>(
    response.task_sessions.map((session: TaskSession) => [session.id, session.updated_at]),
  );
  return response.sessions.map((session) => ({
    sessionId: session.session_id,
    taskId: session.task_id,
    workspaceId: session.workspace_id,
    kind: session.kind,
    name: session.name ?? "",
    agentProfileId: session.agent_profile_id,
    lastActivityAt: updatedAtBySession.get(session.session_id) ?? null,
  }));
}

/** Data and actions for the Quick Chats browse page. */
export function useQuickChatsBrowse() {
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const openQuickChat = useAppStore((state) => state.openQuickChat);
  const removeQuickChatSession = useAppStore((state) => state.removeQuickChatSession);
  const renameQuickChatSession = useAppStore((state) => state.renameQuickChatSession);
  const [items, setItems] = useState<QuickChatBrowseItem[]>([]);
  const [status, setStatus] = useState<"loading" | "ready" | "error">("loading");
  const [sessionToDelete, setSessionToDelete] = useState<string | null>(null);
  const [sessionToRename, setSessionToRename] = useState<QuickChatBrowseItem | null>(null);

  const load = useCallback(async () => {
    if (!workspaceId) return;
    setStatus("loading");
    try {
      setItems(buildBrowseItems(await listQuickChatSessions(workspaceId)));
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
    items,
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
