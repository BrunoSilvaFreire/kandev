import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { UsageTotals } from "@/lib/api/domains/usage-api";
import { CACHE_EXPIRY_MS, CACHE_RECHECK_MS } from "@/lib/usage/efficiency";
import { useSessionUsageInspector } from "./use-session-usage-inspector";

const mocks = vi.hoisted(() => ({
  getSessionUsageTotals: vi.fn(),
  getTaskUsageTotals: vi.fn(),
  state: {
    promptUsage: { bySessionId: {} as Record<string, unknown> },
    messages: { bySession: {} as Record<string, unknown[]> },
  },
}));

vi.mock("@/lib/api/domains/usage-api", () => ({
  getSessionUsageTotals: (...args: unknown[]) => mocks.getSessionUsageTotals(...args),
  getTaskUsageTotals: (...args: unknown[]) => mocks.getTaskUsageTotals(...args),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mocks.state) => unknown) => selector(mocks.state),
}));

vi.mock("@/lib/i18n", () => ({ t: (key: string) => key }));

const TASK_ID = "task-1";

function totals(
  scope: "task" | "session",
  scopeId: string,
  overrides: Partial<UsageTotals> = {},
): UsageTotals {
  return {
    scope,
    scope_id: scopeId,
    tokens_in: 10,
    tokens_cached_read: 20,
    tokens_cached_write: 0,
    tokens_out: 5,
    tokens_thought: 0,
    tokens_total: 35,
    cost_subcents: 100,
    event_count: 2,
    estimated_event_count: 0,
    unpriced_event_count: 0,
    output_tokens_complete: true,
    first_event_at: null,
    last_event_at: null,
    ...overrides,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  mocks.state.promptUsage.bySessionId = {};
  mocks.state.messages.bySession = {};
  mocks.getSessionUsageTotals.mockResolvedValue(totals("session", "session-a"));
  mocks.getTaskUsageTotals.mockResolvedValue(totals("task", TASK_ID));
});

describe("useSessionUsageInspector", () => {
  it("does not fetch without a task id", async () => {
    renderHook(() => useSessionUsageInspector(null, "session-a", { enabled: true }));
    await act(async () => {});
    expect(mocks.getSessionUsageTotals).not.toHaveBeenCalled();
    expect(mocks.getTaskUsageTotals).not.toHaveBeenCalled();
  });

  it("does not fetch while disabled", async () => {
    renderHook(() => useSessionUsageInspector(TASK_ID, "session-a", { enabled: false }));
    await act(async () => {});
    expect(mocks.getSessionUsageTotals).not.toHaveBeenCalled();
  });

  it("fetches session and task totals when enabled", async () => {
    const { result } = renderHook(() =>
      useSessionUsageInspector(TASK_ID, "session-a", { enabled: true }),
    );

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(mocks.getSessionUsageTotals).toHaveBeenCalledWith(TASK_ID, "session-a");
    expect(mocks.getTaskUsageTotals).toHaveBeenCalledWith(TASK_ID);
    expect(result.current.session?.scope).toBe("session");
    expect(result.current.task?.scope).toBe("task");
  });

  it("refetches when the live prompt usage changes", async () => {
    const { rerender } = renderHook(() =>
      useSessionUsageInspector(TASK_ID, "session-a", { enabled: true }),
    );
    await waitFor(() => expect(mocks.getSessionUsageTotals).toHaveBeenCalledTimes(1));

    mocks.state.promptUsage.bySessionId["session-a"] = { inputTokens: 1 };
    rerender();

    await waitFor(() => expect(mocks.getSessionUsageTotals).toHaveBeenCalledTimes(2));
  });

  it("turns likely_expired while idle once the cache window passes", async () => {
    vi.useFakeTimers();
    try {
      mocks.state.messages.bySession["session-a"] = [{ created_at: new Date().toISOString() }];
      mocks.getSessionUsageTotals.mockResolvedValue(
        totals("session", "session-a", { event_count: 3 }),
      );
      const { result } = renderHook(() =>
        useSessionUsageInspector(TASK_ID, "session-a", { enabled: true }),
      );
      await act(async () => {
        await Promise.resolve();
      });
      expect(result.current.status).toBe("warm");

      await act(async () => {
        vi.advanceTimersByTime(CACHE_EXPIRY_MS + CACHE_RECHECK_MS);
      });
      expect(result.current.status).toBe("likely_expired");
    } finally {
      vi.useRealTimers();
    }
  });

  it("ignores an in-flight response after the scope is disabled", async () => {
    let resolveFirst!: (value: UsageTotals) => void;
    mocks.getSessionUsageTotals.mockReturnValueOnce(
      new Promise<UsageTotals>((resolve) => {
        resolveFirst = resolve;
      }),
    );
    const { result, rerender } = renderHook(
      ({ enabled }: { enabled: boolean }) =>
        useSessionUsageInspector(TASK_ID, "session-a", { enabled }),
      { initialProps: { enabled: true } },
    );

    rerender({ enabled: false });
    await act(async () => {
      resolveFirst(totals("session", "session-a"));
    });

    expect(result.current.session).toBeNull();
  });

  it("ignores a stale response after the session changes", async () => {
    let resolveFirst!: (value: UsageTotals) => void;
    mocks.getSessionUsageTotals.mockReturnValueOnce(
      new Promise<UsageTotals>((resolve) => {
        resolveFirst = resolve;
      }),
    );
    mocks.getSessionUsageTotals.mockResolvedValue(totals("session", "session-b"));

    const { result, rerender } = renderHook(
      ({ sessionId }: { sessionId: string }) =>
        useSessionUsageInspector(TASK_ID, sessionId, { enabled: true }),
      { initialProps: { sessionId: "session-a" } },
    );

    rerender({ sessionId: "session-b" });
    await act(async () => {
      resolveFirst(totals("session", "session-a"));
    });

    await waitFor(() => expect(result.current.session?.scope_id).toBe("session-b"));
  });
});
