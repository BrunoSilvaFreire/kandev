import { describe, expect, it } from "vitest";
import type { UsageGroup } from "@/lib/api/domains/usage-api";
import { buildUsageDisplayRows } from "./usage-panel-rows";

function group(overrides: Partial<UsageGroup> = {}): UsageGroup {
  return {
    session_id: "s1",
    agent_profile_id: "profile-1",
    agent_type: "claude",
    model: "model-x",
    provider: "provider-1",
    totals: {
      scope: "group",
      scope_id: "",
      tokens_in: 100,
      tokens_cached_read: 300,
      tokens_cached_write: 0,
      tokens_out: 10,
      tokens_thought: 0,
      tokens_total: 410,
      cost_subcents: 42,
      event_count: 1,
      estimated_event_count: 0,
      unpriced_event_count: 0,
      output_tokens_complete: true,
      first_event_at: null,
      last_event_at: null,
    },
    ...overrides,
  };
}

const BASE_ARGS = {
  sessions: undefined,
  profiles: [{ id: "profile-1", name: "Claude Opus" }],
  deletedProfile: "Deleted profile",
  lastPromptBySession: {},
  messagesBySession: {},
  now: 1_000_000,
  unknown: "Unknown",
};

describe("buildUsageDisplayRows", () => {
  it("rolls groups up by agent and names the profile", () => {
    const rows = buildUsageDisplayRows(
      { ...BASE_ARGS, view: "agent", groups: [group(), group({ model: "model-y" })] },
      "Unknown",
    );

    expect(rows).toHaveLength(1);
    expect(rows[0].primary).toBe("Claude Opus");
    expect(rows[0].totals.event_count).toBe(2);
    expect(rows[0].secondary).toContain("model-x");
    expect(rows[0].secondary).toContain("model-y");
  });

  it("rolls groups up by model", () => {
    const rows = buildUsageDisplayRows(
      { ...BASE_ARGS, view: "model", groups: [group(), group({ agent_profile_id: "profile-2" })] },
      "Unknown",
    );

    expect(rows).toHaveLength(1);
    expect(rows[0].primary).toBe("model-x");
    expect(rows[0].secondary).toBe("provider-1");
  });

  it("merges a session with no ledger rows into the session view", () => {
    const rows = buildUsageDisplayRows(
      {
        ...BASE_ARGS,
        view: "session",
        groups: [group({ session_id: "s1" })],
        sessions: [{ id: "s1", name: "Session one" }, { id: "s2" }],
      },
      "Unknown",
    );

    expect(rows).toHaveLength(2);
    const idle = rows.find((row) => row.sessionId === "s2");
    expect(idle?.totals.event_count).toBe(0);
    expect(idle?.primary).toBe("Unknown");
  });

  it("labels an agent group with no profile id as unknown", () => {
    const rows = buildUsageDisplayRows(
      { ...BASE_ARGS, view: "agent", groups: [group({ agent_profile_id: "" })] },
      "Unknown",
    );

    expect(rows[0].primary).toBe("Unknown");
  });

  it("falls back to the agent profile name for a session that has no name", () => {
    const rows = buildUsageDisplayRows(
      {
        ...BASE_ARGS,
        view: "session",
        groups: [group({ session_id: "1234567890abcdef" })],
        sessions: [{ id: "1234567890abcdef" }],
      },
      "Unknown",
    );

    expect(rows[0].primary).toBe("Claude Opus");
    expect(rows[0].primary).not.toContain("12345678");
    expect(rows[0].title).toBe("12345678");
  });

  it("never renders a deleted profile id as the agent row label", () => {
    const rows = buildUsageDisplayRows(
      {
        ...BASE_ARGS,
        view: "agent",
        groups: [group({ session_id: null, agent_profile_id: "dead-profile-uuid" })],
      },
      "Unknown",
    );

    expect(rows[0].primary).toBe("Deleted profile");
    expect(rows[0].primary).not.toContain("dead-pro");
    expect(rows[0].title).toBe("dead-pro");
  });

  it("keeps a deleted-session group as a null session id", () => {
    const rows = buildUsageDisplayRows(
      { ...BASE_ARGS, view: "session", groups: [group({ session_id: null })] },
      "Unknown",
    );

    expect(rows).toHaveLength(1);
    expect(rows[0].sessionId).toBeNull();
    expect(rows[0].isSession).toBe(true);
  });
});
