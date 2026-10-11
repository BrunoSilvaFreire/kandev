/**
 * Pure model for the Quick Chats browse page: what a row shows and how rows
 * are ordered. Everything here is derived from the chat's own conversation
 * rows as the server reported them, never from the selected workspace.
 */
export type QuickChatBrowseDetail = {
  last_activity_at?: string;
  activity_source?: "message" | "created";
  title_fallback?: string;
  repository_ids?: string[];
  tasks?: { id: string; title: string }[];
  task_count?: number;
};

export type QuickChatBrowseSession = {
  session_id: string;
  task_id: string;
  workspace_id: string;
  kind: "chat" | "config";
  name?: string;
  agent_profile_id?: string;
  browse?: QuickChatBrowseDetail;
};

export type QuickChatBrowseItem = {
  sessionId: string;
  taskId: string;
  workspaceId: string;
  kind: "chat" | "config";
  /** The stored name; empty when nobody named the chat. */
  name: string;
  /** Excerpt of the first user message, shown only when there is no name. */
  titleFallback: string;
  agentProfileId?: string;
  lastActivityAt: string | null;
  /** False when the chat has no messages and the time is its creation. */
  hasMessages: boolean;
  repositoryIds: string[];
  tasks: { id: string; title: string }[];
  /** Created tasks beyond the ones listed. */
  moreTasks: number;
};

export type QuickChatBrowseOrder = "recent" | "oldest";

export function buildQuickChatBrowseItems(
  sessions: readonly QuickChatBrowseSession[],
): QuickChatBrowseItem[] {
  return sessions.map((session) => {
    const detail = session.browse ?? {};
    const tasks = detail.tasks ?? [];
    return {
      sessionId: session.session_id,
      taskId: session.task_id,
      workspaceId: session.workspace_id,
      kind: session.kind,
      name: session.name ?? "",
      titleFallback: detail.title_fallback ?? "",
      agentProfileId: session.agent_profile_id,
      lastActivityAt: detail.last_activity_at ?? null,
      hasMessages: detail.activity_source === "message",
      repositoryIds: detail.repository_ids ?? [],
      tasks,
      moreTasks: Math.max(0, (detail.task_count ?? tasks.length) - tasks.length),
    };
  });
}

/** The stored name wins; the excerpt only fills in for a chat nobody named. */
export function quickChatBrowseTitle(
  item: Pick<QuickChatBrowseItem, "name" | "titleFallback">,
  untitled: string,
): string {
  return item.name.trim() || item.titleFallback.trim() || untitled;
}

function activityTime(item: QuickChatBrowseItem): number | null {
  if (!item.lastActivityAt) return null;
  const time = Date.parse(item.lastActivityAt);
  return Number.isNaN(time) ? null : time;
}

/**
 * Orders rows by activity time. Rows without a usable time sort last in both
 * directions, and equal times fall back to the session ID so the order is
 * stable across reloads.
 */
export function sortQuickChatBrowseItems(
  items: readonly QuickChatBrowseItem[],
  order: QuickChatBrowseOrder,
): QuickChatBrowseItem[] {
  const direction = order === "recent" ? -1 : 1;
  return [...items].sort((left, right) => {
    const leftTime = activityTime(left);
    const rightTime = activityTime(right);
    if (leftTime === null || rightTime === null) {
      if (leftTime === rightTime) return left.sessionId.localeCompare(right.sessionId);
      return leftTime === null ? 1 : -1;
    }
    if (leftTime !== rightTime) return (leftTime - rightTime) * direction;
    return left.sessionId.localeCompare(right.sessionId);
  });
}
