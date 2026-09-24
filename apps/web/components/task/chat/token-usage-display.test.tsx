import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { useSessionContextWindow } from "@/hooks/domains/session/use-session-context-window";
import { useSessionUsageInspector } from "@/hooks/domains/session/use-session-usage-inspector";
import {
  ClarificationEscapeGuardProvider,
  type ClarificationEscapeGuardEntry,
  type ClarificationEscapeGuardRegistry,
} from "@/hooks/use-clarification-escape-guard";
import type { UsageTotals } from "@/lib/api/domains/usage-api";
import { isContextWindowReliable, TokenUsageDisplay } from "./token-usage-display";

const TOOLTIP_ROOT_TESTID = "tooltip-root";

vi.mock("@/hooks/domains/session/use-session-context-window", () => ({
  useSessionContextWindow: vi.fn(),
}));

vi.mock("@/hooks/domains/session/use-session-usage-inspector", () => ({
  useSessionUsageInspector: vi.fn(),
}));

function totals(overrides: Partial<UsageTotals> = {}): UsageTotals {
  return {
    scope: "session",
    scope_id: "sess-1",
    tokens_in: 100,
    tokens_cached_read: 300,
    tokens_cached_write: 20,
    tokens_out: 40,
    tokens_thought: 0,
    tokens_total: 460,
    cost_subcents: 250,
    event_count: 4,
    estimated_event_count: 0,
    unpriced_event_count: 0,
    output_tokens_complete: true,
    first_event_at: null,
    last_event_at: null,
    ...overrides,
  };
}

function inspector(overrides: Partial<ReturnType<typeof useSessionUsageInspector>> = {}) {
  return {
    session: null,
    task: null,
    lastPrompt: undefined,
    status: "unknown" as const,
    expiresAt: null,
    flags: null,
    loading: false,
    error: null,
    ...overrides,
  };
}

beforeEach(() => {
  vi.mocked(useSessionUsageInspector).mockReturnValue(inspector());
});

vi.mock("@kandev/ui/tooltip", () => ({
  TooltipProvider: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  Tooltip: ({ children, open }: { children: React.ReactNode; open?: boolean }) => (
    <div data-testid={TOOLTIP_ROOT_TESTID} data-open={open}>
      {children}
    </div>
  ),
  TooltipTrigger: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function withEscapeGuardRegistry(node: React.ReactNode) {
  const holder: { entry: ClarificationEscapeGuardEntry } = { entry: null };
  const registry: ClarificationEscapeGuardRegistry = {
    register: (_id, predicate) => {
      holder.entry = { test: predicate };
    },
    unregister: () => {
      holder.entry = null;
    },
  };
  render(
    <ClarificationEscapeGuardProvider value={registry}>{node}</ClarificationEscapeGuardProvider>,
  );
  return holder;
}

describe("isContextWindowReliable", () => {
  it("accepts normal usage under the window", () => {
    expect(isContextWindowReliable(200_000, 56_047)).toBe(true);
  });

  it("accepts exactly-full context (100%)", () => {
    expect(isContextWindowReliable(200_000, 200_000)).toBe(true);
  });

  it("rejects impossible usage (used > size) — the wrong-window bug", () => {
    expect(isContextWindowReliable(200_000, 233_900)).toBe(false);
  });

  it("rejects a zero/absent window size", () => {
    expect(isContextWindowReliable(0, 0)).toBe(false);
  });

  it("accepts a correct large window", () => {
    expect(isContextWindowReliable(1_000_000, 233_900)).toBe(true);
  });
});

describe("TokenUsageDisplay", () => {
  it("transitions only the context ring arc", () => {
    vi.mocked(useSessionContextWindow).mockReturnValue({
      size: 200_000,
      used: 56_047,
      remaining: 143_953,
      efficiency: 28,
      compactionCount: 0,
    });

    const { container } = render(<TokenUsageDisplay sessionId="sess-1" />);
    const circles = container.querySelectorAll("circle");
    const usageCircle = circles[1];

    expect(usageCircle).toBeDefined();
    expect(usageCircle.getAttribute("class")).toContain("transition-[stroke-dashoffset]");
    expect(usageCircle.getAttribute("class")).not.toContain("transition-all");
  });

  it("renders nothing when used exceeds size (wrong-window bug)", () => {
    vi.mocked(useSessionContextWindow).mockReturnValue({
      size: 200_000,
      used: 233_900,
      remaining: -33_900,
      efficiency: 117,
      compactionCount: 0,
    });

    const { container } = render(<TokenUsageDisplay sessionId="sess-1" />);

    expect(container.firstChild).toBeNull();
  });

  it("opens the tooltip when the context ring is tapped", () => {
    vi.mocked(useSessionContextWindow).mockReturnValue({
      size: 200_000,
      used: 56_047,
      remaining: 143_953,
      efficiency: 28,
      compactionCount: 0,
    });

    const { getByRole, getByTestId } = render(<TokenUsageDisplay sessionId="sess-1" />);

    fireEvent.click(getByRole("button", { name: /Context window: 28% used/ }));

    expect(getByTestId(TOOLTIP_ROOT_TESTID).getAttribute("data-open")).toBe("true");
    const compactionRow = getByTestId("context-window-compactions-row");
    expect(compactionRow.textContent).toContain("Compactions");
    expect(compactionRow.textContent).toContain("0");
  });

  it("renders an unmeasured pending state when usage is zero", () => {
    vi.mocked(useSessionContextWindow).mockReturnValue({
      size: 200_000,
      used: 0,
      remaining: 200_000,
      efficiency: 0,
      compactionCount: 2,
      source: "acp",
    });

    const { getByRole, getByTestId } = render(<TokenUsageDisplay sessionId="sess-1" />);

    const trigger = getByRole("button", { name: /Context window: usage not measured/ });
    fireEvent.click(trigger);

    expect(getByTestId(TOOLTIP_ROOT_TESTID).getAttribute("data-open")).toBe("true");
    const usage = getByTestId("context-window-usage");
    expect(usage.textContent).toContain("N/A%");
    expect(usage.textContent).toContain("N/A of 200.0K tokens");
    expect(usage.textContent).toContain("Usage data appears after the first completed turn.");
    expect(usage.textContent).not.toContain("0%");
    expect(usage.textContent).not.toContain("0 of");
    expect(usage.textContent).toContain("ACP");
    expect(usage.textContent).toContain("Compactions");
  });

  it("closes a tapped tooltip when Escape is pressed", () => {
    vi.mocked(useSessionContextWindow).mockReturnValue({
      size: 200_000,
      used: 56_047,
      remaining: 143_953,
      efficiency: 28,
      compactionCount: 0,
    });

    const { getByRole, getByTestId } = render(<TokenUsageDisplay sessionId="sess-1" />);

    fireEvent.click(getByRole("button", { name: /Context window: 28% used/ }));
    fireEvent.keyDown(document, { key: "Escape" });

    expect(getByTestId(TOOLTIP_ROOT_TESTID).getAttribute("data-open")).toBe("false");
  });

  it("claims Escape at dialog capture time while the tooltip is pinned", () => {
    vi.mocked(useSessionContextWindow).mockReturnValue({
      size: 200_000,
      used: 56_047,
      remaining: 143_953,
      efficiency: 28,
      compactionCount: 0,
    });

    const holder = withEscapeGuardRegistry(<TokenUsageDisplay sessionId="sess-1" />);
    const trigger = screen.getByRole("button", { name: /Context window: 28% used/ });

    fireEvent.click(trigger);

    expect(holder.entry?.test(new KeyboardEvent("keydown", { key: "Escape" }))).toBe(true);
  });
});

describe("TokenUsageDisplay context source", () => {
  it("labels agent-reported ACP data", () => {
    vi.mocked(useSessionContextWindow).mockReturnValue({
      size: 200_000,
      used: 56_047,
      remaining: 143_953,
      efficiency: 28,
      compactionCount: 2,
      source: "acp",
    });

    const { container, getAllByTestId, getByText, getByLabelText } = render(
      <TokenUsageDisplay sessionId="sess-1" />,
    );

    const tokenCount = getByText("56.0K of 200.0K tokens");
    const source = getByText("ACP");

    expect(container.querySelector(".cursor-help")).not.toBeNull();
    expect(container.querySelector("svg")).not.toBeNull();
    expect(tokenCount.closest('[data-testid="context-window-token-row"]')).toBe(
      source.closest('[data-testid="context-window-token-row"]'),
    );
    expect(getAllByTestId(TOOLTIP_ROOT_TESTID)).toHaveLength(1);
    expect(getByLabelText("About context window source")).toBeDefined();
    expect(getByText(/ACP is the active session's effective window/i)).toBeDefined();
  });

  it("shows the inferred compaction count and its accuracy disclosure", () => {
    vi.mocked(useSessionContextWindow).mockReturnValue({
      size: 200_000,
      used: 56_047,
      remaining: 143_953,
      efficiency: 28,
      compactionCount: 2,
      source: "acp",
    });

    const { container, getByLabelText, getByText } = render(
      <TokenUsageDisplay sessionId="sess-1" />,
    );

    expect(getByText("Compactions")).toBeDefined();
    expect(getByText("2", { selector: "span" })).toBeDefined();
    expect(getByLabelText("About inferred compaction count")).toBeDefined();
    expect(container.textContent).toContain(
      "Kandev infers this count from observed context usage drops",
    );

    const helpButton = getByLabelText("About inferred compaction count");
    const helpId = helpButton.getAttribute("aria-describedby");
    if (!helpId) throw new Error("Expected compaction help to be described");
    const help = document.getElementById(helpId);
    if (!help) throw new Error("Expected compaction help element");
    expect(help.className).toContain("opacity-0");

    fireEvent.click(helpButton);
    expect(help.className).toContain("opacity-100");
    fireEvent.click(helpButton);
    expect(help.className).toContain("opacity-0");
  });

  it("labels model API fallback data", () => {
    vi.mocked(useSessionContextWindow).mockReturnValue({
      size: 128_000,
      used: 64_000,
      remaining: 64_000,
      efficiency: 50,
      compactionCount: 0,
      source: "api",
    });

    const { getByText, getByLabelText } = render(<TokenUsageDisplay sessionId="sess-1" />);

    expect(getByText("API")).toBeDefined();
    const helpButton = getByLabelText("About context window source");
    const helpId = helpButton.getAttribute("aria-describedby");
    if (!helpId) throw new Error("Expected source help to be described");
    const help = document.getElementById(helpId);
    if (!help) throw new Error("Expected source help element");

    fireEvent.click(helpButton);
    expect(help.className).toContain("opacity-100");
    fireEvent.click(helpButton);
    expect(help.className).toContain("opacity-0");
    expect(getByText(/model's advertised maximum from the catalogue/i)).toBeDefined();
  });
});

// eslint-disable-next-line max-lines-per-function -- the badge and inspector scenarios share one harness.
describe("TokenUsageDisplay usage inspector", () => {
  it("renders the cache status badge dot", () => {
    vi.mocked(useSessionContextWindow).mockReturnValue({
      size: 200_000,
      used: 56_047,
      remaining: 143_953,
      efficiency: 28,
      compactionCount: 0,
    });
    vi.mocked(useSessionUsageInspector).mockReturnValue(
      inspector({ status: "warm", session: totals(), task: totals({ scope: "task" }) }),
    );

    render(<TokenUsageDisplay sessionId="sess-1" taskId="task-1" />);

    const dot = screen.getByTestId("usage-cache-status-dot");
    expect(dot.getAttribute("data-cache-status")).toBe("warm");
    expect(dot.className).toContain("bg-green-500");
  });

  it("keeps the quick-chat indicator free of the cache badge", () => {
    vi.mocked(useSessionContextWindow).mockReturnValue({
      size: 200_000,
      used: 56_047,
      remaining: 143_953,
      efficiency: 28,
      compactionCount: 0,
    });
    vi.mocked(useSessionUsageInspector).mockReturnValue(
      inspector({ status: "warm", session: totals() }),
    );

    render(<TokenUsageDisplay sessionId="sess-1" />);

    expect(screen.queryByTestId("usage-cache-status-dot")).toBeNull();
    expect(screen.queryByTestId("usage-inspector")).toBeNull();
    const trigger = screen.getByTestId(TOOLTIP_ROOT_TESTID).querySelector("button")!;
    expect(trigger.getAttribute("aria-label")).not.toContain("Prompt cache");
  });

  it("renders the inspector sections with session and task totals", () => {
    vi.mocked(useSessionContextWindow).mockReturnValue({
      size: 200_000,
      used: 56_047,
      remaining: 143_953,
      efficiency: 28,
      compactionCount: 0,
    });
    vi.mocked(useSessionUsageInspector).mockReturnValue(
      inspector({ status: "warm", session: totals(), task: totals({ scope: "task" }) }),
    );

    render(<TokenUsageDisplay sessionId="sess-1" taskId="task-1" />);
    fireEvent.click(screen.getByTestId(TOOLTIP_ROOT_TESTID).querySelector("button")!);

    expect(screen.getByTestId("usage-inspector-cache")).toBeTruthy();
    expect(screen.getByTestId("usage-inspector-tokens").textContent).toContain("Cached read");
    expect(screen.getByTestId("usage-inspector-cost").textContent).toContain("$0.03");
  });

  it("marks estimated cost with a tilde and shows per-column notes", () => {
    vi.mocked(useSessionContextWindow).mockReturnValue({
      size: 200_000,
      used: 56_047,
      remaining: 143_953,
      efficiency: 28,
      compactionCount: 0,
    });
    vi.mocked(useSessionUsageInspector).mockReturnValue(
      inspector({
        status: "warm",
        session: totals({ estimated_event_count: 2, unpriced_event_count: 1 }),
        task: totals({
          scope: "task",
          estimated_event_count: 0,
          unpriced_event_count: 3,
        }),
      }),
    );

    render(<TokenUsageDisplay sessionId="sess-1" taskId="task-1" />);
    fireEvent.click(screen.getByTestId(TOOLTIP_ROOT_TESTID).querySelector("button")!);

    const cost = screen.getByTestId("usage-inspector-cost");
    expect(cost.textContent).toContain("~$0.03");
    expect(cost.textContent).toContain("estimated");
    expect(cost.textContent).toContain("1 prompt unpriced");
    expect(cost.textContent).toContain("3 prompts unpriced");
  });

  it("tints the trigger when a usage flag is raised", () => {
    vi.mocked(useSessionContextWindow).mockReturnValue({
      size: 200_000,
      used: 56_047,
      remaining: 143_953,
      efficiency: 28,
      compactionCount: 0,
    });
    vi.mocked(useSessionUsageInspector).mockReturnValue(
      inspector({
        status: "likely_expired",
        session: totals(),
        flags: { lowHitRatio: false, highCost: true, cacheLikelyExpired: true },
      }),
    );

    render(<TokenUsageDisplay sessionId="sess-1" taskId="task-1" />);

    const trigger = screen.getByTestId(TOOLTIP_ROOT_TESTID).querySelector("button")!;
    expect(trigger.className).toContain("bg-amber-500/10");
  });

  it("renders the indicator from usage alone when the context window is unreliable", () => {
    vi.mocked(useSessionContextWindow).mockReturnValue({
      size: 200_000,
      used: 233_900,
      remaining: -33_900,
      efficiency: 117,
      compactionCount: 0,
    });
    vi.mocked(useSessionUsageInspector).mockReturnValue(
      inspector({ status: "warm", session: totals(), task: totals({ scope: "task" }) }),
    );

    const { container } = render(<TokenUsageDisplay sessionId="sess-1" taskId="task-1" />);

    expect(container.firstChild).not.toBeNull();
    expect(screen.queryByTestId("context-window-usage")).toBeNull();
    expect(screen.getByTestId("usage-inspector-cache")).toBeTruthy();
  });

  it("shows a no-usage line when the agent reported no events", () => {
    vi.mocked(useSessionContextWindow).mockReturnValue({
      size: 200_000,
      used: 56_047,
      remaining: 143_953,
      efficiency: 28,
      compactionCount: 0,
    });
    vi.mocked(useSessionUsageInspector).mockReturnValue(
      inspector({
        status: "unknown",
        session: totals({ event_count: 0 }),
        task: totals({ scope: "task", event_count: 0 }),
      }),
    );

    render(<TokenUsageDisplay sessionId="sess-1" taskId="task-1" />);
    fireEvent.click(screen.getByTestId(TOOLTIP_ROOT_TESTID).querySelector("button")!);

    expect(screen.getByTestId("usage-inspector-no-usage")).toBeTruthy();
  });
});
