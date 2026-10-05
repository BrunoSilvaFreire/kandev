import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { act, cleanup, renderHook } from "@testing-library/react";
import type { MessageSearchHit } from "@/lib/api/domains/session-api";

const mockSessionSearch = vi.fn();
const mockTaskSearch = vi.fn();

vi.mock("@/lib/api/domains/session-api", () => ({
  searchSessionMessages: (...args: unknown[]) => mockSessionSearch(...args),
  searchTaskMessages: (...args: unknown[]) => mockTaskSearch(...args),
}));

import { useSessionSearch } from "./use-session-search";

function makeHit(id: string, sessionId?: string): MessageSearchHit {
  return {
    id,
    author_type: "agent",
    type: "text",
    snippet: id,
    created_at: new Date().toISOString(),
    session_id: sessionId,
  };
}

async function flush() {
  await Promise.resolve();
  await Promise.resolve();
}

beforeEach(() => {
  vi.useFakeTimers();
  mockSessionSearch.mockReset();
  mockTaskSearch.mockReset();
});

afterEach(() => {
  vi.useRealTimers();
  cleanup();
});

function renderTaskSearch(sessionId = "sess-1", activate = vi.fn()) {
  return renderHook(() =>
    useSessionSearch(sessionId, undefined, undefined, {
      taskId: "task-1",
      activeSessionId: "sess-1",
      activateSession: activate,
    }),
  );
}

async function searchFor(result: { current: { open: () => void; setQuery: (q: string) => void } }) {
  act(() => {
    result.current.open();
    result.current.setQuery("needle");
  });
  await act(async () => {
    vi.advanceTimersByTime(250);
    await flush();
  });
}

describe("useSessionSearch task scope", () => {
  it("searches the whole task and pages with the opaque cursor", async () => {
    mockTaskSearch
      .mockResolvedValueOnce({ hits: [makeHit("m1")], total: 1, has_more: true, next_cursor: "c1" })
      .mockResolvedValueOnce({ hits: [makeHit("m2")], total: 1, has_more: false });
    const { result } = renderTaskSearch();

    await searchFor(result);
    expect(mockTaskSearch).toHaveBeenCalledWith("task-1", "sess-1", "needle", { limit: 50 });
    expect(mockSessionSearch).not.toHaveBeenCalled();
    expect(result.current.taskScoped).toBe(true);
    expect(result.current.hasMore).toBe(true);

    await act(async () => {
      result.current.loadMore();
      await flush();
    });
    expect(mockTaskSearch).toHaveBeenLastCalledWith("task-1", "sess-1", "needle", {
      limit: 50,
      cursor: "c1",
    });
    expect(result.current.hits.map((hit) => hit.id)).toEqual(["m1", "m2"]);
    expect(result.current.hasMore).toBe(false);
  });

  it("keeps same-session selection immediate", async () => {
    mockTaskSearch.mockResolvedValueOnce({
      hits: [makeHit("m1", "sess-1")],
      total: 1,
      has_more: false,
    });
    const activate = vi.fn();
    const { result } = renderTaskSearch("sess-1", activate);
    await searchFor(result);

    act(() => {
      result.current.setActiveHit("m1");
    });
    expect(activate).not.toHaveBeenCalled();
    expect(result.current.activeHitId).toBe("m1");
  });
});

describe("useSessionSearch cross-session", () => {
  it("activates another session and resumes the hit once it is current", async () => {
    mockTaskSearch.mockResolvedValueOnce({
      hits: [makeHit("m2", "sess-2")],
      total: 1,
      has_more: false,
    });
    const activate = vi.fn();
    const taskScope = { taskId: "task-1", activeSessionId: "sess-1", activateSession: activate };
    const { result, rerender } = renderHook(
      ({ sessionId }: { sessionId: string }) =>
        useSessionSearch(sessionId, undefined, undefined, taskScope),
      { initialProps: { sessionId: "sess-1" } },
    );
    await searchFor(result);
    expect(result.current.hits[0]?.id).toBe("m2");

    act(() => {
      result.current.setActiveHit("m2");
    });
    // Cross-session: activation is requested, and the hit is not yet marked.
    expect(activate).toHaveBeenCalledWith("sess-2");
    expect(result.current.activeHitId).toBeNull();

    rerender({ sessionId: "sess-2" });
    await act(async () => {
      await flush();
    });
    // Once the target session is current, the pending hit resumes.
    expect(result.current.activeHitId).toBe("m2");
  });
});
