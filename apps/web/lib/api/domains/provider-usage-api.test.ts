import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://api.test" }),
}));

import {
  fetchProviderUsage,
  fetchUsageBreakdown,
  fetchUsageIndexStatus,
  startUsageIndex,
  updateUsageSources,
} from "./provider-usage-api";

const fetchSpy = vi.fn<typeof fetch>();
const API_BASE_URL = "http://api.test";

beforeEach(() => {
  fetchSpy.mockReset();
  vi.stubGlobal("fetch", fetchSpy);
});

afterEach(() => vi.unstubAllGlobals());

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

describe("provider usage API", () => {
  it("requests the overview for the given range", async () => {
    fetchSpy.mockResolvedValue(jsonResponse({ range: "24h", providers: [] }));

    const overview = await fetchProviderUsage("24h");

    const [input] = fetchSpy.mock.calls[0] ?? [];
    expect(String(input)).toBe(`${API_BASE_URL}/api/v1/provider-usage?range=24h`);
    expect(overview.range).toBe("24h");
  });

  it("reads and starts the index job", async () => {
    fetchSpy.mockImplementation(() =>
      Promise.resolve(jsonResponse({ state: "never", files_total: 0, files_done: 0 })),
    );

    await fetchUsageIndexStatus();
    let [, init] = fetchSpy.mock.calls[0] ?? [];
    expect(init?.method).toBeUndefined();

    await startUsageIndex();
    [, init] = fetchSpy.mock.calls[1] ?? [];
    expect(init?.method).toBe("POST");
  });

  it("puts source toggles and returns the updated sources", async () => {
    fetchSpy.mockResolvedValue(
      jsonResponse({ sources: [{ source: "codex_local", enabled: true, toggleable: true }] }),
    );

    const sources = await updateUsageSources({ codex_local: true });

    const [input, init] = fetchSpy.mock.calls[0] ?? [];
    expect(String(input)).toBe(`${API_BASE_URL}/api/v1/provider-usage/sources`);
    expect(init?.method).toBe("PUT");
    expect(JSON.parse(String(init?.body))).toEqual({ codex_local: true });
    expect(sources).toEqual([{ source: "codex_local", enabled: true, toggleable: true }]);
  });

  it("builds the breakdown query and omits empty values", async () => {
    fetchSpy.mockResolvedValue(jsonResponse({ range: "7d", group_by: "session", rows: [] }));

    await fetchUsageBreakdown({
      range: "7d",
      groupBy: "session",
      sort: "tokens",
      order: "desc",
      limit: 50,
      offset: 0,
      provider: "",
      model: undefined,
    });

    const [input] = fetchSpy.mock.calls[0] ?? [];
    const url = new URL(String(input));
    expect(url.pathname).toBe("/api/v1/provider-usage/breakdown");
    expect(url.searchParams.get("group_by")).toBe("session");
    expect(url.searchParams.get("limit")).toBe("50");
    expect(url.searchParams.has("provider")).toBe(false);
    expect(url.searchParams.has("model")).toBe(false);
    // Display-only drill-down labels are never sent.
    expect(url.searchParams.has("sessionLabel")).toBe(false);
  });

  it("propagates a failed request", async () => {
    fetchSpy.mockResolvedValue(new Response(JSON.stringify({ error: "bad" }), { status: 400 }));
    await expect(fetchProviderUsage("7d")).rejects.toThrow();
  });
});
