import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://api.test" }),
}));

import {
  getSessionUsageTotals,
  getTaskUsageBreakdown,
  getTaskUsageTotals,
  type UsageTotals,
} from "./usage-api";

const fetchSpy = vi.fn<typeof fetch>();
const API_BASE_URL = "http://api.test";

const TOTALS: UsageTotals = {
  scope: "session",
  scope_id: "session-1",
  tokens_in: 10,
  tokens_cached_read: 20,
  tokens_cached_write: 5,
  tokens_out: 3,
  tokens_thought: 0,
  tokens_total: 38,
  cost_subcents: 123,
  event_count: 4,
  estimated_event_count: 1,
  unpriced_event_count: 0,
  output_tokens_complete: true,
  first_event_at: null,
  last_event_at: null,
};

beforeEach(() => {
  fetchSpy.mockReset();
  vi.stubGlobal("fetch", fetchSpy);
});

afterEach(() => vi.unstubAllGlobals());

describe("usage API", () => {
  it("fetches task totals from the task usage route", async () => {
    fetchSpy.mockResolvedValue(
      new Response(JSON.stringify({ ...TOTALS, scope: "task", scope_id: "task-1" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    const totals = await getTaskUsageTotals("task-1");

    expect(String(fetchSpy.mock.calls[0]?.[0])).toBe(`${API_BASE_URL}/api/v1/tasks/task-1/usage`);
    expect(totals.scope).toBe("task");
  });

  it("fetches session totals from the session usage route", async () => {
    fetchSpy.mockResolvedValue(
      new Response(JSON.stringify(TOTALS), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    const totals = await getSessionUsageTotals("task-1", "session-1");

    expect(String(fetchSpy.mock.calls[0]?.[0])).toBe(
      `${API_BASE_URL}/api/v1/tasks/task-1/sessions/session-1/usage`,
    );
    expect(totals.first_event_at).toBeNull();
    expect(totals.cost_subcents).toBe(123);
  });

  it("fetches the breakdown with a null deleted-session id", async () => {
    fetchSpy.mockResolvedValue(
      new Response(
        JSON.stringify({
          task_id: "task-1",
          task: { ...TOTALS, scope: "task", scope_id: "task-1" },
          groups: [
            {
              session_id: null,
              agent_profile_id: "profile-1",
              agent_type: "claude",
              model: "model-x",
              provider: "provider-1",
              totals: { ...TOTALS, scope: "group", scope_id: "" },
            },
          ],
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ),
    );

    const breakdown = await getTaskUsageBreakdown("task-1");

    expect(String(fetchSpy.mock.calls[0]?.[0])).toBe(
      `${API_BASE_URL}/api/v1/tasks/task-1/usage/breakdown`,
    );
    expect(breakdown.task_id).toBe("task-1");
    expect(breakdown.groups).toHaveLength(1);
    expect(breakdown.groups[0].session_id).toBeNull();
  });

  it("parses an empty breakdown groups array", async () => {
    fetchSpy.mockResolvedValue(
      new Response(
        JSON.stringify({
          task_id: "task-1",
          task: { ...TOTALS, scope: "task", scope_id: "task-1" },
          groups: [],
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ),
    );

    const breakdown = await getTaskUsageBreakdown("task-1");

    expect(breakdown.groups).toEqual([]);
  });

  it("propagates a failed request", async () => {
    fetchSpy.mockResolvedValue(
      new Response(JSON.stringify({ error: "task not found" }), {
        status: 404,
        headers: { "Content-Type": "application/json" },
      }),
    );

    await expect(getTaskUsageTotals("missing")).rejects.toThrow();
  });
});
