import { cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/lib/api/domains/provider-usage-api", () => ({
  fetchProviderUsage: vi.fn(),
}));

import { fetchProviderUsage } from "@/lib/api/domains/provider-usage-api";
import { useProviderUsage } from "./use-provider-usage";
import type { ProviderUsageRange } from "@/lib/types/provider-usage";

const fetchProviderUsageMock = vi.mocked(fetchProviderUsage);

beforeEach(() => fetchProviderUsageMock.mockReset());
afterEach(() => cleanup());

function overview(range: ProviderUsageRange) {
  return {
    range,
    truncated: false,
    indexing: { state: "done" as const, files_total: 0, files_done: 0 },
    sources: [],
    providers: [],
  };
}

describe("useProviderUsage", () => {
  it("reads on mount and exposes the overview", async () => {
    fetchProviderUsageMock.mockResolvedValue(overview("7d"));
    const { result } = renderHook(() => useProviderUsage("7d"));

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(fetchProviderUsageMock).toHaveBeenCalledWith("7d");
    expect(result.current.overview?.range).toBe("7d");
    expect(result.current.error).toBe(false);
  });

  it("refetches on refresh and range change", async () => {
    fetchProviderUsageMock.mockResolvedValue(overview("7d"));
    const { result, rerender } = renderHook(
      ({ range }: { range: ProviderUsageRange }) => useProviderUsage(range),
      { initialProps: { range: "7d" } },
    );
    await waitFor(() => expect(result.current.loading).toBe(false));

    result.current.refresh();
    await waitFor(() => expect(fetchProviderUsageMock).toHaveBeenCalledTimes(2));

    rerender({ range: "30d" });
    await waitFor(() => expect(fetchProviderUsageMock).toHaveBeenLastCalledWith("30d"));
  });
});
