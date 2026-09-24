import { renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const fetchProfileUtilizationMock = vi.fn();

vi.mock("@/lib/api/domains/agent-profile-utilization-api", () => ({
  fetchProfileUtilization: (...args: unknown[]) => fetchProfileUtilizationMock(...args),
}));

import {
  chunkProfileIds,
  useAgentProfileUtilization,
} from "./use-agent-profile-utilization";

afterEach(() => {
  vi.clearAllMocks();
});

describe("chunkProfileIds", () => {
  it("splits at the 50-id endpoint limit", () => {
    const ids = Array.from({ length: 120 }, (_, i) => `p${i}`);
    const batches = chunkProfileIds(ids);
    expect(batches.map((batch) => batch.length)).toEqual([50, 50, 20]);
    expect(batches.flat()).toEqual(ids);
  });
});

describe("useAgentProfileUtilization", () => {
  it("never fetches while disabled or without ids", () => {
    const { rerender } = renderHook(
      ({ ids, enabled }: { ids: string[]; enabled: boolean }) =>
        useAgentProfileUtilization(ids, enabled),
      { initialProps: { ids: ["a"], enabled: false } },
    );
    expect(fetchProfileUtilizationMock).not.toHaveBeenCalled();

    rerender({ ids: [], enabled: true });
    expect(fetchProfileUtilizationMock).not.toHaveBeenCalled();
  });

  it("loads items when enabled", async () => {
    fetchProfileUtilizationMock.mockResolvedValue([
      { profile_id: "a", state: "known", remaining_pct: 72 },
    ]);

    const { result } = renderHook(() => useAgentProfileUtilization(["a"], true));

    await waitFor(() => expect(result.current.items.a?.state).toBe("known"));
    expect(fetchProfileUtilizationMock).toHaveBeenCalledWith(["a"], {
      init: { signal: expect.any(AbortSignal) },
    });
  });

  it("batches more than 50 ids into separate requests", async () => {
    fetchProfileUtilizationMock.mockResolvedValue([]);
    const ids = Array.from({ length: 120 }, (_, i) => `p${i}`);

    const { result } = renderHook(() => useAgentProfileUtilization(ids, true));

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(fetchProfileUtilizationMock).toHaveBeenCalledTimes(3);
    const sizes = fetchProfileUtilizationMock.mock.calls.map(
      (call) => (call[0] as string[]).length,
    );
    expect(sizes).toEqual([50, 50, 20]);
  });

  it("marks a failed batch's candidates unavailable", async () => {
    fetchProfileUtilizationMock.mockRejectedValue(new Error("boom"));

    const { result } = renderHook(() => useAgentProfileUtilization(["a"], true));

    await waitFor(() => expect(result.current.error).toBeTruthy());
    expect(result.current.items.a?.state).toBe("unavailable");
  });

  it("aborts requests on unmount", () => {
    fetchProfileUtilizationMock.mockReturnValue(
      new Promise(() => undefined),
    );
    const errorSpy = vi.spyOn(console, "error").mockImplementation(() => undefined);

    const { unmount } = renderHook(() => useAgentProfileUtilization(["a"], true));
    const signal = fetchProfileUtilizationMock.mock.calls[0]?.[1]?.init?.signal as AbortSignal;
    unmount();

    expect(signal.aborted).toBe(true);
    expect(errorSpy).not.toHaveBeenCalled();
    errorSpy.mockRestore();
  });
});
