import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/lib/api/domains/provider-usage-api", () => ({
  fetchUsageBreakdown: vi.fn(),
}));

import { fetchUsageBreakdown } from "@/lib/api/domains/provider-usage-api";
import { useUsageBreakdown } from "./use-usage-breakdown";
import type { ProviderUsageRange, UsageBreakdownResponse } from "@/lib/types/provider-usage";

const fetchMock = vi.mocked(fetchUsageBreakdown);

beforeEach(() => fetchMock.mockReset());
afterEach(() => cleanup());

function response(range: ProviderUsageRange = "7d"): UsageBreakdownResponse {
  return {
    range,
    group_by: "session",
    total_rows: 0,
    total_tokens: 0,
    total_cost_subcents: 0,
    total_events: 0,
    facets: { providers: [], models: [], agent_types: [] },
    rows: [],
  };
}

describe("useUsageBreakdown", () => {
  it("reads the default session view on mount", async () => {
    fetchMock.mockResolvedValue(response());
    const { result } = renderHook(() => useUsageBreakdown("7d"));

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(fetchMock).toHaveBeenCalledWith(
      expect.objectContaining({ range: "7d", groupBy: "session", sort: "tokens", order: "desc" }),
    );
    expect(result.current.data?.group_by).toBe("session");
    expect(result.current.error).toBe(false);
  });

  it("resets the page when the group changes", async () => {
    fetchMock.mockResolvedValue(response());
    const { result } = renderHook(() => useUsageBreakdown("7d"));
    await waitFor(() => expect(result.current.loading).toBe(false));

    act(() => result.current.setQuery({ offset: 40 }));
    act(() => result.current.setQuery({ groupBy: "model" }));

    await waitFor(() =>
      expect(fetchMock).toHaveBeenLastCalledWith(
        expect.objectContaining({ groupBy: "model", offset: 0 }),
      ),
    );
  });

  it("debounces search and applies it once", async () => {
    fetchMock.mockResolvedValue(response());
    const { result } = renderHook(() => useUsageBreakdown("7d"));
    await waitFor(() => expect(result.current.loading).toBe(false));
    const calls = fetchMock.mock.calls.length;

    act(() => result.current.setQuery({ q: "alpha", offset: 40 }));
    expect(fetchMock.mock.calls.length).toBe(calls);

    await waitFor(() => expect(fetchMock.mock.calls.length).toBe(calls + 1), { timeout: 2_000 });
    expect(fetchMock.mock.calls.at(-1)?.[0]).toMatchObject({ q: "alpha", offset: 0 });
  });

  it("refetches on refresh", async () => {
    fetchMock.mockResolvedValue(response());
    const { result } = renderHook(() => useUsageBreakdown("7d"));
    await waitFor(() => expect(result.current.loading).toBe(false));
    const calls = fetchMock.mock.calls.length;

    act(() => result.current.refresh());
    await waitFor(() => expect(fetchMock.mock.calls.length).toBe(calls + 1));
  });
});
