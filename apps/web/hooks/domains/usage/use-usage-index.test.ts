import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const fetchUsageIndexStatusMock = vi.fn();
const startUsageIndexMock = vi.fn();
vi.mock("@/lib/api/domains/provider-usage-api", () => ({
  fetchUsageIndexStatus: (...args: unknown[]) => fetchUsageIndexStatusMock(...args),
  startUsageIndex: (...args: unknown[]) => startUsageIndexMock(...args),
}));

import { useUsageIndex } from "./use-usage-index";

beforeEach(() => {
  vi.useFakeTimers();
  fetchUsageIndexStatusMock.mockReset();
  startUsageIndexMock.mockReset();
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

async function flush() {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(0);
  });
}

describe("useUsageIndex", () => {
  it("auto-starts a never-run index", async () => {
    fetchUsageIndexStatusMock.mockResolvedValue({ state: "never", files_total: 0, files_done: 0 });
    startUsageIndexMock.mockResolvedValue({ state: "running", files_total: 2, files_done: 0 });

    const { result } = renderHook(() => useUsageIndex());
    await flush();

    expect(startUsageIndexMock).toHaveBeenCalledTimes(1);
    expect(result.current.status?.state).toBe("running");
  });

  it("polls only while running, stops at done, and notifies onFinished", async () => {
    fetchUsageIndexStatusMock.mockResolvedValue({
      state: "running",
      files_total: 2,
      files_done: 0,
    });
    const onFinished = vi.fn();
    const { result } = renderHook(() => useUsageIndex({ onFinished }));
    await flush();
    expect(result.current.status?.state).toBe("running");

    fetchUsageIndexStatusMock.mockResolvedValue({ state: "done", files_total: 2, files_done: 2 });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(2_000);
    });
    expect(result.current.status?.state).toBe("done");
    expect(onFinished).toHaveBeenCalledTimes(1);

    const callsAtDone = fetchUsageIndexStatusMock.mock.calls.length;
    await act(async () => {
      await vi.advanceTimersByTimeAsync(6_000);
    });
    expect(fetchUsageIndexStatusMock.mock.calls.length).toBe(callsAtDone);
  });

  it("reindex starts a run on demand", async () => {
    fetchUsageIndexStatusMock.mockResolvedValue({ state: "done", files_total: 1, files_done: 1 });
    startUsageIndexMock.mockResolvedValue({ state: "running", files_total: 1, files_done: 0 });
    const { result } = renderHook(() => useUsageIndex());
    await flush();

    await act(async () => {
      result.current.reindex();
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(startUsageIndexMock).toHaveBeenCalledTimes(1);
  });
});
