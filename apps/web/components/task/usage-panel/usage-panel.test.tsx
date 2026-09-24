import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { UsageTotals } from "@/lib/api/domains/usage-api";

const mocks = vi.hoisted(() => ({
  breakdown: {
    task: null as UsageTotals | null,
    groups: [] as unknown[],
    lastPromptBySession: {},
    loading: false,
    error: null as string | null,
    now: 0,
  },
  isMobile: false,
  state: {
    taskSessionsByTask: {
      itemsByTaskId: {} as Record<string, Array<{ id: string; name?: string }>>,
    },
    agentProfiles: { items: [{ id: "profile-1", name: "Claude Opus" }] },
    messages: { bySession: {} as Record<string, unknown[]> },
  },
}));

vi.mock("@/hooks/domains/session/use-task-usage-breakdown", () => ({
  useTaskUsageBreakdown: () => mocks.breakdown,
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mocks.state) => unknown) => selector(mocks.state),
}));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: mocks.isMobile }),
}));

import { UsagePanel } from "./usage-panel";

const TASK_ID = "task-1";
const TABLE_TEST_ID = "usage-panel-table";
const ROW_TEST_ID = "usage-panel-row";

function totals(overrides: Partial<UsageTotals> = {}): UsageTotals {
  return {
    scope: "task",
    scope_id: TASK_ID,
    tokens_in: 100,
    tokens_cached_read: 300,
    tokens_cached_write: 0,
    tokens_out: 20,
    tokens_thought: 0,
    tokens_total: 420,
    cost_subcents: 250,
    event_count: 2,
    estimated_event_count: 0,
    unpriced_event_count: 0,
    output_tokens_complete: true,
    first_event_at: null,
    last_event_at: null,
    ...overrides,
  };
}

function groupTotal(overrides: Partial<UsageTotals> = {}) {
  return {
    session_id: "s1",
    agent_profile_id: "profile-1",
    agent_type: "claude",
    model: "model-x",
    provider: "provider-1",
    totals: totals({ scope: "group", scope_id: "", ...overrides }),
  };
}

beforeEach(() => {
  mocks.isMobile = false;
  mocks.breakdown = {
    task: null,
    groups: [],
    lastPromptBySession: {},
    loading: false,
    error: null,
    now: 0,
  };
  mocks.state.taskSessionsByTask.itemsByTaskId = {};
});

afterEach(cleanup);

describe("UsagePanel", () => {
  it("shows an empty state without a task", () => {
    render(<UsagePanel taskId={null} />);
    expect(screen.getByTestId("usage-panel-empty")).toBeTruthy();
  });

  it("renders the task totals and the agent table", () => {
    mocks.breakdown = {
      ...mocks.breakdown,
      task: totals(),
      groups: [groupTotal()],
    };

    render(<UsagePanel taskId={TASK_ID} />);

    expect(screen.getByTestId("usage-panel-totals")).toBeTruthy();
    expect(screen.getByTestId(TABLE_TEST_ID)).toBeTruthy();
    expect(screen.getByTestId(ROW_TEST_ID).textContent).toContain("Claude Opus");
  });

  it("switches to the session view and merges sessions with no usage", () => {
    mocks.state.taskSessionsByTask.itemsByTaskId[TASK_ID] = [
      { id: "s1", name: "Session one" },
      { id: "s2" },
    ];
    mocks.breakdown = {
      ...mocks.breakdown,
      task: totals(),
      groups: [groupTotal()],
    };

    render(<UsagePanel taskId={TASK_ID} />);
    fireEvent.click(screen.getByTestId("usage-panel-view-session"));

    const rows = screen.getAllByTestId(ROW_TEST_ID);
    expect(rows).toHaveLength(2);
    expect(rows.some((row) => row.textContent?.includes("s2"))).toBe(true);
  });

  it("gives the table a name header so header and row cells line up", () => {
    mocks.breakdown = { ...mocks.breakdown, task: totals(), groups: [groupTotal()] };

    render(<UsagePanel taskId={TASK_ID} />);

    const table = screen.getByTestId(TABLE_TEST_ID);
    const headerCells = table.querySelectorAll("thead th");
    const firstRowCells = table.querySelectorAll("tbody tr:first-child td");
    expect(headerCells.length).toBe(firstRowCells.length);
    expect(headerCells[0].textContent).toBe("Name");
  });

  it("renders the per-session cache status, expiry and last prompt", () => {
    mocks.state.taskSessionsByTask.itemsByTaskId[TASK_ID] = [{ id: "s1", name: "Session one" }];
    mocks.breakdown = {
      ...mocks.breakdown,
      task: totals(),
      groups: [groupTotal()],
      lastPromptBySession: { s1: { inputTokens: 5, outputTokens: 1, totalTokens: 6 } },
    };

    render(<UsagePanel taskId={TASK_ID} />);
    fireEvent.click(screen.getByTestId("usage-panel-view-session"));

    const table = screen.getByTestId(TABLE_TEST_ID);
    expect(table.textContent).toContain("Cache");
    expect(table.textContent).toContain("Estimated expiry");
    expect(table.textContent).toContain("Last prompt");
    expect(table.textContent).toContain("Unknown");
  });

  it("shows the Part B per-row flags", () => {
    mocks.breakdown = {
      ...mocks.breakdown,
      task: totals(),
      groups: [groupTotal({ cost_subcents: 60_000, event_count: 3 })],
    };

    render(<UsagePanel taskId={TASK_ID} />);

    expect(screen.getByTestId("usage-panel-row-flags").textContent).toContain("High session cost");
  });

  it("renders stacked cards below the mobile boundary", () => {
    mocks.isMobile = true;
    mocks.breakdown = { ...mocks.breakdown, task: totals(), groups: [groupTotal()] };

    render(<UsagePanel taskId={TASK_ID} />);

    expect(screen.getByTestId("usage-panel-cards")).toBeTruthy();
    expect(screen.queryByTestId(TABLE_TEST_ID)).toBeNull();
  });
});
