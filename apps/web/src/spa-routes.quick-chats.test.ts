import { describe, expect, it } from "vitest";
import { resolveSpaRoute } from "./spa-routes";

describe("resolveSpaRoute quick chats", () => {
  it("resolves the browse page", () => {
    expect(resolveSpaRoute("/quick-chats", new URLSearchParams())).toEqual({
      kind: "quickChats",
    });
  });

  it("resolves a Quick Chat detail page as a quick-chat task surface", () => {
    expect(resolveSpaRoute("/quick-chats/task-1", new URLSearchParams())).toEqual({
      kind: "taskDetail",
      taskId: "task-1",
      surface: "quick-chat",
      sessionId: undefined,
      layout: null,
      simple: undefined,
      mode: undefined,
      panel: undefined,
    });
  });

  it("carries the requested panel into the detail route", () => {
    const route = resolveSpaRoute("/quick-chats/task-1", new URLSearchParams({ panel: "plan" }));
    expect(route).toMatchObject({ kind: "taskDetail", taskId: "task-1", panel: "plan" });
  });

  it("does not treat a nested quick-chat path as a detail route", () => {
    expect(resolveSpaRoute("/quick-chats/task-1/extra", new URLSearchParams()).kind).toBe("kanban");
  });
});
