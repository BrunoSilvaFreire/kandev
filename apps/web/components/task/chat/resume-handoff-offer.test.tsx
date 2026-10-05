import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { UsageTotals } from "@/lib/api/domains/usage-api";
import type { Message, TaskSession } from "@/lib/types/http";

const mocks = vi.hoisted(() => ({
  run: vi.fn(),
  clearError: vi.fn(),
  hook: { isRunning: false, error: null as string | null },
  usage: { session: null as UsageTotals | null },
  state: {
    taskSessions: { items: {} as Record<string, TaskSession | undefined> },
    messages: { bySession: {} as Record<string, Message[] | undefined> },
    sessionAgentctl: { itemsBySessionId: {} as Record<string, { status: string } | undefined> },
    userSettings: {
      defaultUtilityAgentProfileId: "profile-1" as string | null,
      defaultUtilityAgentId: null as string | null,
    },
  },
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mocks.state) => unknown) => selector(mocks.state),
}));

vi.mock("@/hooks/domains/session/use-session-usage-inspector", () => ({
  useSessionUsageInspector: () => ({
    session: mocks.usage.session,
    task: null,
    lastPrompt: undefined,
    status: "unknown",
    expiresAt: null,
    flags: null,
    loading: false,
    error: null,
  }),
}));

vi.mock("@/hooks/domains/session/use-resume-with-handoff", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("@/hooks/domains/session/use-resume-with-handoff")>();
  return {
    ...actual,
    useResumeWithHandoff: () => ({
      run: mocks.run,
      clearError: () => {
        mocks.clearError();
        mocks.hook.error = null;
      },
      ...mocks.hook,
    }),
  };
});

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

import { ResumeHandoffOffer } from "./resume-handoff-offer";

const SESSION_ID = "session-1";
const TASK_ID = "task-1";
const OFFER_TEST_ID = "resume-handoff-offer";
const ACTION_TEST_ID = "resume-handoff-action";
const DRAFT_TEXT = "extra instructions";
const OLD = new Date(Date.now() - 2 * 60 * 60 * 1000).toISOString();

function sessionTotals(lastEventAt: string | null): UsageTotals {
  return {
    scope: "session",
    scope_id: SESSION_ID,
    tokens_in: 10,
    tokens_cached_read: 20,
    tokens_cached_write: 0,
    tokens_out: 5,
    tokens_thought: 0,
    tokens_total: 35,
    cost_subcents: 100,
    event_count: 3,
    estimated_event_count: 0,
    unpriced_event_count: 0,
    output_tokens_complete: true,
    first_event_at: lastEventAt,
    last_event_at: lastEventAt,
  };
}

function message(overrides: Partial<Message>): Message {
  return {
    id: "message-1",
    session_id: SESSION_ID as Message["session_id"],
    task_id: TASK_ID as Message["task_id"],
    author_type: "agent",
    content: "done",
    type: "message",
    created_at: OLD,
    ...overrides,
  };
}

function setSession(overrides: Partial<TaskSession> = {}) {
  mocks.state.taskSessions.items[SESSION_ID] = {
    id: SESSION_ID,
    task_id: TASK_ID,
    state: "WAITING_FOR_INPUT",
    started_at: OLD,
    updated_at: OLD,
    ...overrides,
  } as TaskSession;
}

function setMessages(messages: Message[]) {
  mocks.state.messages.bySession[SESSION_ID] = messages;
}

function renderOffer() {
  return render(<ResumeHandoffOffer taskId={TASK_ID} sessionId={SESSION_ID} />);
}

beforeEach(() => {
  vi.clearAllMocks();
  mocks.hook.isRunning = false;
  mocks.hook.error = null;
  mocks.state.taskSessions.items = {};
  mocks.state.messages.bySession = {};
  mocks.state.sessionAgentctl.itemsBySessionId = {};
  mocks.state.userSettings.defaultUtilityAgentProfileId = "profile-1";
  mocks.state.userSettings.defaultUtilityAgentId = null;
  mocks.usage.session = sessionTotals(OLD);
  mocks.run.mockResolvedValue(true);
});

afterEach(cleanup);

describe("ResumeHandoffOffer", () => {
  it("offers a resume handoff for a settled idle session with an expired cache", () => {
    setSession();
    setMessages([message({})]);

    renderOffer();

    expect(screen.getByTestId(OFFER_TEST_ID)).toBeTruthy();
  });

  it("hides the offer while the agent is busy", () => {
    setSession({ state: "RUNNING" });
    setMessages([message({})]);

    renderOffer();

    expect(screen.queryByTestId(OFFER_TEST_ID)).toBeNull();
  });

  it("hides the offer while a clarification is pending", () => {
    setSession({ pending_action: "clarification" });
    setMessages([message({})]);

    renderOffer();

    expect(screen.queryByTestId(OFFER_TEST_ID)).toBeNull();
  });

  it("hides the offer when no utility agent is configured", () => {
    setSession();
    setMessages([message({})]);
    mocks.state.userSettings.defaultUtilityAgentProfileId = null;
    mocks.state.userSettings.defaultUtilityAgentId = null;

    renderOffer();

    expect(screen.queryByTestId(OFFER_TEST_ID)).toBeNull();
  });

  it("hides the offer when the agentctl status is an error", () => {
    setSession();
    setMessages([message({})]);
    mocks.state.sessionAgentctl.itemsBySessionId[SESSION_ID] = { status: "error" };

    renderOffer();

    expect(screen.queryByTestId(OFFER_TEST_ID)).toBeNull();
  });

  it("stays visible with its error after a failed run makes the session ineligible", () => {
    // A successful reset posts a context-reset message that re-dates the newest
    // message, so the offer would otherwise disappear with the error attached.
    setSession();
    setMessages([message({ created_at: new Date().toISOString() })]);
    mocks.hook.error = "send failed";

    renderOffer();

    expect(screen.getByTestId(OFFER_TEST_ID)).toBeTruthy();
    expect(screen.getByTestId("resume-handoff-error")).toBeTruthy();
  });

  it("disables the action when a failure left the session ineligible", () => {
    setSession({ state: "RUNNING" });
    setMessages([message({ created_at: new Date().toISOString() })]);
    mocks.hook.error = "send failed";

    renderOffer();

    expect(screen.getByTestId(OFFER_TEST_ID)).toBeTruthy();
    expect(screen.getByTestId(ACTION_TEST_ID)).toHaveProperty("disabled", true);
  });

  it("dismisses the offer and its error", () => {
    setSession();
    setMessages([message({ created_at: new Date().toISOString() })]);
    mocks.hook.error = "send failed";

    renderOffer();
    expect(screen.getByTestId(OFFER_TEST_ID)).toBeTruthy();

    fireEvent.click(screen.getByTestId("resume-handoff-dismiss"));

    expect(mocks.clearError).toHaveBeenCalledTimes(1);
    expect(screen.queryByTestId(OFFER_TEST_ID)).toBeNull();
  });

  it("hides the offer when the last usage event is inside the cache window", () => {
    setSession();
    setMessages([message({})]);
    mocks.usage.session = sessionTotals(new Date().toISOString());

    renderOffer();

    expect(screen.queryByTestId(OFFER_TEST_ID)).toBeNull();
  });
});

describe("ResumeHandoffOffer - actions and draft handling", () => {
  it("runs the resume flow when the action is clicked", () => {
    setSession();
    setMessages([message({})]);

    renderOffer();
    fireEvent.click(screen.getByTestId(ACTION_TEST_ID));

    expect(mocks.run).toHaveBeenCalledTimes(1);
    expect(mocks.run).toHaveBeenCalledWith("");
  });

  it("displays draft hint when getDraft returns non-empty draft", () => {
    setSession();
    setMessages([message({})]);

    render(
      <ResumeHandoffOffer taskId={TASK_ID} sessionId={SESSION_ID} getDraft={() => DRAFT_TEXT} />,
    );

    expect(screen.getByTestId("resume-handoff-draft-hint")).toBeTruthy();
    expect(screen.getByTestId("resume-handoff-draft-hint").textContent).toBe(
      "task:resumeHandoffIncludesDraft",
    );
  });

  it("passes draft to run and clears draft on success", async () => {
    setSession();
    setMessages([message({})]);
    mocks.run.mockResolvedValue(true);
    const clearDraft = vi.fn();

    render(
      <ResumeHandoffOffer
        taskId={TASK_ID}
        sessionId={SESSION_ID}
        getDraft={() => DRAFT_TEXT}
        clearDraft={clearDraft}
      />,
    );

    await fireEvent.click(screen.getByTestId(ACTION_TEST_ID));

    expect(mocks.run).toHaveBeenCalledWith(DRAFT_TEXT);
    expect(clearDraft).toHaveBeenCalledTimes(1);
  });

  it("does not clear draft when run returns false", async () => {
    setSession();
    setMessages([message({})]);
    mocks.run.mockResolvedValue(false);
    const clearDraft = vi.fn();

    render(
      <ResumeHandoffOffer
        taskId={TASK_ID}
        sessionId={SESSION_ID}
        getDraft={() => DRAFT_TEXT}
        clearDraft={clearDraft}
      />,
    );

    await fireEvent.click(screen.getByTestId(ACTION_TEST_ID));

    expect(mocks.run).toHaveBeenCalledWith(DRAFT_TEXT);
    expect(clearDraft).not.toHaveBeenCalled();
  });

  it("stays dismissed until a newer message re-arms the offer", () => {
    setSession();
    setMessages([message({})]);

    const { rerender } = renderOffer();
    fireEvent.click(screen.getByTestId("resume-handoff-dismiss"));
    expect(screen.queryByTestId(OFFER_TEST_ID)).toBeNull();

    rerender(<ResumeHandoffOffer taskId={TASK_ID} sessionId={SESSION_ID} />);
    expect(screen.queryByTestId(OFFER_TEST_ID)).toBeNull();

    setMessages([
      message({}),
      message({
        id: "message-2",
        created_at: new Date(Date.now() - 90 * 60 * 1000).toISOString(),
      }),
    ]);
    rerender(<ResumeHandoffOffer taskId={TASK_ID} sessionId={SESSION_ID} />);
    expect(screen.getByTestId(OFFER_TEST_ID)).toBeTruthy();
  });
});
