import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { getTaskTransitionSummary, listTaskActivity } from "@/lib/api/domains/task-activity-api";
import { useStepVisits, useTaskTransitionSummary } from "./use-task-activity";

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({ connection: { status: "connected" } }),
}));

vi.mock("@/lib/api/domains/task-activity-api", () => ({
  getTaskTransitionSummary: vi.fn(),
  listTaskActivity: vi.fn(),
}));

const summary = vi.mocked(getTaskTransitionSummary);
const activity = vi.mocked(listTaskActivity);

beforeEach(() => {
  summary.mockReset();
  activity.mockReset();
});

afterEach(() => vi.clearAllMocks());

describe("useTaskTransitionSummary", () => {
  it("indexes committed counts and per-step visits", async () => {
    summary.mockResolvedValueOnce({
      counts: [{ from_step_id: "a", to_step_id: "b", count: 4 }],
      visits: [
        {
          step_id: "b",
          count: 2,
          sessions: [{ session_id: "sess-1", occurred_at: "2026-09-25T10:00:00Z" }],
        },
      ],
    });
    const { result } = renderHook(() => useTaskTransitionSummary("task-1"));
    await waitFor(() => expect(result.current.status).toBe("success"));
    expect(result.current.counts["a->b"]).toBe(4);
    expect(result.current.visits.b).toEqual({
      count: 2,
      sessions: [{ session_id: "sess-1", occurred_at: "2026-09-25T10:00:00Z" }],
    });
  });

  it("stays idle without a task", () => {
    const { result } = renderHook(() => useTaskTransitionSummary(null));
    expect(result.current.status).toBe("idle");
    expect(summary).not.toHaveBeenCalled();
  });
});

describe("useStepVisits", () => {
  function event(id: string) {
    return {
      kind: "transition",
      id,
      occurred_at: "2026-07-01T00:00:00Z",
      transition: { id: Number(id), trigger: "turn_complete", actor_kind: "system" },
    };
  }

  it("pages step visits with the opaque cursor", async () => {
    activity
      .mockResolvedValueOnce({ events: [event("1")], has_more: true, next_cursor: "c1" } as never)
      .mockResolvedValueOnce({ events: [event("2")], has_more: false } as never);
    const { result } = renderHook(() => useStepVisits("task-1", "step-b"));
    await waitFor(() => expect(result.current.status).toBe("success"));
    expect(result.current.visits.map((visit) => visit.id)).toEqual(["1"]);
    expect(result.current.hasMore).toBe(true);

    result.current.loadMore();
    await waitFor(() => expect(result.current.visits).toHaveLength(2));
    expect(activity).toHaveBeenLastCalledWith("task-1", { stepId: "step-b", cursor: "c1" });
  });

  it("does not fetch without a step", () => {
    renderHook(() => useStepVisits("task-1", null));
    expect(activity).not.toHaveBeenCalled();
  });
});
