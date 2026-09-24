import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://api.test" }),
}));

import { fetchProfileUtilization } from "./agent-profile-utilization-api";

const fetchSpy = vi.fn<typeof fetch>();
const API_BASE_URL = "http://api.test";

beforeEach(() => {
  fetchSpy.mockReset();
  vi.stubGlobal("fetch", fetchSpy);
});

afterEach(() => vi.unstubAllGlobals());

describe("agent profile utilization API", () => {
  it("posts the requested ids and returns deterministic items", async () => {
    fetchSpy.mockResolvedValue(
      new Response(
        JSON.stringify({
          profiles: [
            { profile_id: "b", state: "unknown" },
            { profile_id: "a", state: "known", remaining_pct: 72 },
          ],
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ),
    );

    const items = await fetchProfileUtilization(["a", "b"]);

    const [input, init] = fetchSpy.mock.calls[0] ?? [];
    expect(String(input)).toBe(`${API_BASE_URL}/api/v1/agent-profiles/utilization`);
    expect(init?.method).toBe("POST");
    expect(JSON.parse(String(init?.body))).toEqual({ profile_ids: ["a", "b"] });
    expect(items).toEqual([
      { profile_id: "b", state: "unknown" },
      { profile_id: "a", state: "known", remaining_pct: 72 },
    ]);
  });

  it("propagates a failed request", async () => {
    fetchSpy.mockResolvedValue(
      new Response(JSON.stringify({ error: "invalid payload" }), {
        status: 400,
        headers: { "Content-Type": "application/json" },
      }),
    );

    await expect(fetchProfileUtilization(["a"])).rejects.toThrow();
  });
});
