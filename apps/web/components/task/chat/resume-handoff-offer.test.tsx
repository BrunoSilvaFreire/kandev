import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { Message, TaskSession } from "@/lib/types/http";

const mocks = vi.hoisted(() => ({
  run: vi.fn(),
  clearError: vi.fn(),
  hook: { isRunning: false, error: null as string | null },
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
const OLD = new Date(Date.now() - 2 * 60 * 60 * 1000).toISOString();

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
    expect(screen.getByTestId("resume-handoff-action")).toHaveProperty("disabled", true);
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

  it("hides the offer when the newest message is inside the cache window", () => {
    setSession();
    setMessages([message({ created_at: new Date().toISOString() })]);

    renderOffer();

    expect(screen.queryByTestId(OFFER_TEST_ID)).toBeNull();
  });

  it("runs the resume flow when the action is clicked", () => {
    setSession();
    setMessages([message({})]);

    renderOffer();
    fireEvent.click(screen.getByTestId("resume-handoff-action"));

    expect(mocks.run).toHaveBeenCalledTimes(1);
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
