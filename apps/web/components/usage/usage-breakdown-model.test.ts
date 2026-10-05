import { describe, expect, it } from "vitest";
import type { UsageBreakdownRow } from "@/lib/types/provider-usage";
import {
  activeFilterChips,
  clearFilters,
  drillDown,
  removeFilter,
  sharePct,
} from "./usage-breakdown-model";

const SESSION = "session";
const ALPHA_SESSION = "Alpha session";

function row(overrides: Partial<UsageBreakdownRow> = {}): UsageBreakdownRow {
  return {
    key: "key",
    label: "label",
    task_id: "",
    session_name: "",
    provider: "",
    model: "",
    models: 1,
    tokens_total: 0,
    tokens_in: 0,
    tokens_out: 0,
    tokens_cached_read: 0,
    cost_subcents: 0,
    events: 0,
    first_at: "2026-09-28T00:00:00Z",
    last_at: "2026-09-28T00:00:00Z",
    ...overrides,
  };
}

describe("drillDown", () => {
  it("narrows provider to model", () => {
    const next = drillDown({ groupBy: "provider", offset: 30 }, row({ key: "anthropic" }));
    expect(next).toMatchObject({ groupBy: "model", provider: "anthropic", offset: 0 });
  });

  it("narrows model to session", () => {
    const next = drillDown({ groupBy: "model" }, row({ key: "claude-sonnet" }));
    expect(next).toMatchObject({ groupBy: SESSION, model: "claude-sonnet" });
  });

  it("narrows task to session and keeps the label", () => {
    const next = drillDown({ groupBy: "task" }, row({ key: "task-1", label: "Alpha" }));
    expect(next).toMatchObject({ groupBy: SESSION, taskId: "task-1", taskLabel: "Alpha" });
  });

  it("narrows a session to its models", () => {
    const next = drillDown({ groupBy: SESSION }, row({ key: "sess-1", label: ALPHA_SESSION }));
    expect(next).toMatchObject({
      groupBy: "model",
      sessionId: "sess-1",
      sessionLabel: ALPHA_SESSION,
    });
  });

  it("falls back to the session view for a day bucket", () => {
    const next = drillDown({ groupBy: "day" }, row({ key: "2026-09-28" }));
    expect(next).toMatchObject({ groupBy: SESSION });
  });
});

describe("activeFilterChips", () => {
  it("lists every active filter", () => {
    const chips = activeFilterChips({
      provider: "anthropic",
      model: "claude-sonnet",
      agentType: "claude-acp",
      taskId: "task-1",
      taskLabel: "Alpha",
      sessionId: "sess-1",
      sessionLabel: ALPHA_SESSION,
      q: "refactor",
    });
    expect(chips).toHaveLength(6);
    expect(chips.find((chip) => chip.key === "sessionId")?.value).toBe(ALPHA_SESSION);
  });

  it("omits empty filters", () => {
    expect(activeFilterChips({})).toEqual([]);
  });
});

describe("removeFilter", () => {
  it("clears the filter and its label", () => {
    const next = removeFilter(
      { sessionId: "sess-1", sessionLabel: "Alpha", offset: 40 },
      "sessionId",
    );
    expect(next.sessionId).toBeUndefined();
    expect(next.sessionLabel).toBeUndefined();
    expect(next.offset).toBe(0);
  });

  it("clearFilters drops them all", () => {
    expect(clearFilters({ provider: "a", model: "b", q: "c", taskId: "d" })).toEqual({
      offset: 0,
    });
  });
});

describe("sharePct", () => {
  it("clamps to the valid range", () => {
    expect(sharePct(50, 100)).toBe(50);
    expect(sharePct(0, 0)).toBe(0);
    expect(sharePct(500, 100)).toBe(100);
  });
});
