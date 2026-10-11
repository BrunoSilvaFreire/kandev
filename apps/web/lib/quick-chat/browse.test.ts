import { describe, expect, it } from "vitest";
import {
  buildQuickChatBrowseItems,
  quickChatBrowseTitle,
  sortQuickChatBrowseItems,
  type QuickChatBrowseSession,
} from "./browse";

const SAME_TIME = "2026-10-01T10:00:00Z";

function session(id: string, browse?: QuickChatBrowseSession["browse"], name?: string) {
  return {
    session_id: id,
    task_id: `task-${id}`,
    workspace_id: "ws",
    kind: "chat" as const,
    name,
    browse,
  };
}

describe("quick chat browse model", () => {
  it("orders by activity with a stable tie-break and undated rows last either way", () => {
    const items = buildQuickChatBrowseItems([
      session("b", { last_activity_at: SAME_TIME, activity_source: "message" }),
      session("undated"),
      session("a", { last_activity_at: SAME_TIME, activity_source: "message" }),
      session("new", { last_activity_at: "2026-10-09T08:00:00Z", activity_source: "message" }),
      session("broken", { last_activity_at: "not a date" }),
      session("old", { last_activity_at: "2026-09-01T08:00:00Z", activity_source: "created" }),
    ]);
    const ids = (order: "recent" | "oldest") =>
      sortQuickChatBrowseItems(items, order).map((item) => item.sessionId);

    expect(ids("recent")).toEqual(["new", "a", "b", "old", "broken", "undated"]);
    expect(ids("oldest")).toEqual(["old", "a", "b", "new", "broken", "undated"]);
  });

  it("never lets the excerpt replace a stored name", () => {
    const [named, excerpted, bare] = buildQuickChatBrowseItems([
      session("1", { title_fallback: "why does the importer fail" }, "Release notes"),
      session("2", { title_fallback: "why does the importer fail" }),
      session("3"),
    ]);
    expect(quickChatBrowseTitle(named, "Untitled")).toBe("Release notes");
    expect(quickChatBrowseTitle(excerpted, "Untitled")).toBe("why does the importer fail");
    expect(quickChatBrowseTitle(bare, "Untitled")).toBe("Untitled");
  });

  it("marks a chat with no messages and counts the tasks that were not listed", () => {
    const [empty, busy] = buildQuickChatBrowseItems([
      session("1", { last_activity_at: SAME_TIME, activity_source: "created" }),
      session("2", {
        last_activity_at: SAME_TIME,
        activity_source: "message",
        repository_ids: ["repo-1"],
        tasks: [{ id: "t1", title: "One" }],
        task_count: 4,
      }),
    ]);
    expect(empty.hasMessages).toBe(false);
    expect(empty.repositoryIds).toEqual([]);
    expect(busy).toMatchObject({ hasMessages: true, repositoryIds: ["repo-1"], moreTasks: 3 });
  });

  it("shows no links when the server sent no browse detail", () => {
    const [item] = buildQuickChatBrowseItems([session("1")]);
    expect(item).toMatchObject({
      lastActivityAt: null,
      tasks: [],
      moreTasks: 0,
      repositoryIds: [],
    });
  });
});
