import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { UsageBreakdown } from "@/lib/api/domains/usage-api";
import { CACHE_RECHECK_MS } from "@/lib/usage/efficiency";
import { useTaskUsageBreakdown } from "./use-task-usage-breakdown";

const mocks = vi.hoisted(() => ({
  getTaskUsageBreakdown: vi.fn(),
  state: {
    taskSessionsByTask: { itemsByTaskId: {} as Record<string, Array<{ id: string }>> },
    promptUsage: { bySessionId: {} as Record<string, unknown> },
  },
}));

vi.mock("@/lib/api/domains/usage-api", () => ({
  getTaskUsageBreakdown: (...args: unknown[]) => mocks.getTaskUsageBreakdown(...args),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mocks.state) => unknown) => selector(mocks.state),
}));

vi.mock("@/lib/i18n", () => ({ t: (key: string) => key }));

function breakdown(taskId: string): UsageBreakdown {
  return {
    task_id: taskId,
    task: {
      scope: "task",
      scope_id: taskId,
      tokens_in: 0,
      tokens_cached_read: 0,
      tokens_cached_write: 0,
      tokens_out: 0,
      tokens_thought: 0,
      tokens_total: 0,
      cost_subcents: 0,
      event_count: 0,
      estimated_event_count: 0,
      unpriced_event_count: 0,
      output_tokens_complete: true,
      first_event_at: null,
      last_event_at: null,
    },
    groups: [],
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  mocks.state.taskSessionsByTask.itemsByTaskId = {};
  mocks.state.promptUsage.bySessionId = {};
  mocks.getTaskUsageBreakdown.mockImplementation((taskId: string) =>
    Promise.resolve(breakdown(taskId)),
  );
});

describe("useTaskUsageBreakdown", () => {
  it("fetches the breakdown on mount", async () => {
    const { result } = renderHook(() => useTaskUsageBreakdown("task-1", { enabled: true }));

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(mocks.getTaskUsageBreakdown).toHaveBeenCalledWith("task-1");
    expect(result.current.task?.scope_id).toBe("task-1");
  });

  it("does not fetch while disabled", async () => {
    renderHook(() => useTaskUsageBreakdown("task-1", { enabled: false }));
    await act(async () => {});
    expect(mocks.getTaskUsageBreakdown).not.toHaveBeenCalled();
  });

  it("refetches when one of this task's sessions reports new prompt usage", async () => {
    mocks.state.taskSessionsByTask.itemsByTaskId["task-1"] = [{ id: "s1" }];
    const { rerender } = renderHook(() => useTaskUsageBreakdown("task-1", { enabled: true }));
    await waitFor(() => expect(mocks.getTaskUsageBreakdown).toHaveBeenCalledTimes(1));

    mocks.state.promptUsage.bySessionId = {
      s1: { inputTokens: 1, outputTokens: 0, totalTokens: 1 },
    };
    rerender();

    await waitFor(() => expect(mocks.getTaskUsageBreakdown).toHaveBeenCalledTimes(2));
  });

  it("does not refetch for another task's session", async () => {
    mocks.state.taskSessionsByTask.itemsByTaskId["task-1"] = [{ id: "s1" }];
    const { rerender } = renderHook(() => useTaskUsageBreakdown("task-1", { enabled: true }));
    await waitFor(() => expect(mocks.getTaskUsageBreakdown).toHaveBeenCalledTimes(1));

    mocks.state.promptUsage.bySessionId = {
      "other-session": { inputTokens: 9, outputTokens: 0, totalTokens: 9 },
    };
    rerender();

    await act(async () => {});
    expect(mocks.getTaskUsageBreakdown).toHaveBeenCalledTimes(1);
  });

  it("ignores a stale response after the task changes", async () => {
    let resolveFirst!: (value: UsageBreakdown) => void;
    mocks.getTaskUsageBreakdown.mockReturnValueOnce(
      new Promise<UsageBreakdown>((resolve) => {
        resolveFirst = resolve;
      }),
    );
    mocks.getTaskUsageBreakdown.mockImplementation((taskId: string) =>
      Promise.resolve(breakdown(taskId)),
    );

    const { result, rerender } = renderHook(
      ({ taskId }: { taskId: string }) => useTaskUsageBreakdown(taskId, { enabled: true }),
      { initialProps: { taskId: "task-1" } },
    );

    rerender({ taskId: "task-2" });
    await act(async () => {
      resolveFirst(breakdown("task-1"));
    });

    await waitFor(() => expect(result.current.task?.scope_id).toBe("task-2"));
    expect(result.current.task?.scope_id).not.toBe("task-1");
  });

  it("advances the cache clock on the shared tick without fetching", async () => {
    vi.useFakeTimers();
    try {
      const { result } = renderHook(() => useTaskUsageBreakdown("task-1", { enabled: true }));
      await act(async () => {
        await Promise.resolve();
      });
      const before = result.current.now;
      expect(mocks.getTaskUsageBreakdown).toHaveBeenCalledTimes(1);

      await act(async () => {
        vi.advanceTimersByTime(CACHE_RECHECK_MS);
      });

      expect(result.current.now).toBeGreaterThan(before);
      expect(mocks.getTaskUsageBreakdown).toHaveBeenCalledTimes(1);
    } finally {
      vi.useRealTimers();
    }
  });
});
