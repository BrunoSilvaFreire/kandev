import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { WebSocketUnavailableError } from "@/lib/api/domains/session-reset-api";
import {
  CACHE_EXPIRY_MS,
  isResumeHandoffCandidate,
  useResumeWithHandoff,
} from "./use-resume-with-handoff";

const mocks = vi.hoisted(() => ({
  runTranscriptUtility: vi.fn(),
  requestContextReset: vi.fn(),
  sendMessageRequest: vi.fn(),
  clearContextWindow: vi.fn(),
  addMessage: vi.fn(),
  order: [] as string[],
}));

vi.mock("@/hooks/use-summarize-session", () => ({
  runTranscriptUtility: (...args: unknown[]) => {
    mocks.order.push("extract");
    return mocks.runTranscriptUtility(...args);
  },
}));

vi.mock("@/lib/api/domains/session-reset-api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/domains/session-reset-api")>();
  return {
    WebSocketUnavailableError: actual.WebSocketUnavailableError,
    requestContextReset: (...args: unknown[]) => {
      mocks.order.push("reset");
      return mocks.requestContextReset(...args);
    },
  };
});

vi.mock("@/hooks/message-request", () => ({
  sendMessageRequest: (...args: unknown[]) => {
    mocks.order.push("send");
    return mocks.sendMessageRequest(...args);
  },
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (
    selector: (state: {
      clearContextWindow: typeof mocks.clearContextWindow;
      addMessage: typeof mocks.addMessage;
    }) => unknown,
  ) =>
    selector({
      clearContextWindow: mocks.clearContextWindow,
      addMessage: mocks.addMessage,
    }),
}));

vi.mock("@/lib/i18n", () => ({
  t: (key: string) => key,
}));

const TASK_ID = "task-1";
const SESSION_ID = "session-1";
const HANDOFF_TEXT = "handoff text";
const AGENT_BUSY = "agent busy";
const RESUME_FAILED_ERROR = "task:resumeHandoffFailed";
const EXTRACT_FAILED_ERROR = "task:resumeHandoffCouldNotExtract";
const HANDOFF_NOT_SENT_ERROR = "task:resumeHandoffNotSent";

function candidateInput(overrides: Partial<Parameters<typeof isResumeHandoffCandidate>[0]> = {}) {
  return {
    state: "WAITING_FOR_INPUT" as const,
    isAgentBusy: false,
    hasLiveExecution: true,
    hasAgentMessage: true,
    newestMessageAt: 1_000,
    now: 1_000 + CACHE_EXPIRY_MS,
    ...overrides,
  };
}

describe("isResumeHandoffCandidate", () => {
  it("accepts a settled, idle, live session whose cache has expired", () => {
    expect(isResumeHandoffCandidate(candidateInput())).toBe(true);
  });

  it("accepts exactly at the expiry boundary", () => {
    expect(isResumeHandoffCandidate(candidateInput())).toBe(true);
  });

  it("rejects one millisecond before the expiry boundary", () => {
    expect(isResumeHandoffCandidate(candidateInput({ now: 1_000 + CACHE_EXPIRY_MS - 1 }))).toBe(
      false,
    );
  });

  it.each([
    ["state is not waiting for input", { state: "RUNNING" as const }],
    ["the agent is busy", { isAgentBusy: true }],
    ["there is no live execution", { hasLiveExecution: false }],
    ["there is no agent message", { hasAgentMessage: false }],
    ["there is no newest message", { newestMessageAt: null }],
  ])("rejects when %s", (_label, overrides) => {
    expect(isResumeHandoffCandidate(candidateInput(overrides))).toBe(false);
  });
});

// eslint-disable-next-line max-lines-per-function -- the ordering and failure scenarios share one hook harness.
describe("useResumeWithHandoff", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    mocks.order.length = 0;
    mocks.runTranscriptUtility.mockResolvedValue({ status: "ok", text: HANDOFF_TEXT });
    mocks.requestContextReset.mockResolvedValue(undefined);
    mocks.sendMessageRequest.mockResolvedValue({ id: "msg-1", session_id: SESSION_ID });
  });

  it("extracts, resets, clears the context window, then sends the handoff", async () => {
    const { result } = renderHook(() => useResumeWithHandoff(TASK_ID, SESSION_ID));

    await act(async () => {
      await result.current.run();
    });

    expect(mocks.order).toEqual(["extract", "reset", "send"]);
    expect(mocks.runTranscriptUtility).toHaveBeenCalledWith(
      SESSION_ID,
      "builtin-extract-resume-handoff",
    );
    expect(mocks.requestContextReset).toHaveBeenCalledWith(SESSION_ID);
    expect(mocks.clearContextWindow).toHaveBeenCalledWith(SESSION_ID);
    expect(mocks.sendMessageRequest).toHaveBeenCalledWith({
      taskId: TASK_ID,
      resolvedSessionId: SESSION_ID,
      finalMessage: HANDOFF_TEXT,
      modelToSend: undefined,
      planMode: false,
    });
    expect(mocks.addMessage).toHaveBeenCalledWith({ id: "msg-1", session_id: SESSION_ID });
    expect(result.current.error).toBeNull();
    expect(result.current.isRunning).toBe(false);
  });

  it("does not reset or send when extraction returns no text", async () => {
    mocks.runTranscriptUtility.mockResolvedValue({ status: "failed", error: "extract failed" });
    const { result } = renderHook(() => useResumeWithHandoff(TASK_ID, SESSION_ID));

    await act(async () => {
      await result.current.run();
    });

    expect(mocks.order).toEqual(["extract"]);
    expect(mocks.requestContextReset).not.toHaveBeenCalled();
    expect(mocks.clearContextWindow).not.toHaveBeenCalled();
    expect(mocks.sendMessageRequest).not.toHaveBeenCalled();
    expect(result.current.error).toBe(EXTRACT_FAILED_ERROR);
  });

  it("does not reset when the transcript is empty", async () => {
    mocks.runTranscriptUtility.mockResolvedValue({ status: "empty" });
    const { result } = renderHook(() => useResumeWithHandoff(TASK_ID, SESSION_ID));

    await act(async () => {
      await result.current.run();
    });

    expect(mocks.requestContextReset).not.toHaveBeenCalled();
    expect(mocks.sendMessageRequest).not.toHaveBeenCalled();
    expect(result.current.error).toBe(EXTRACT_FAILED_ERROR);
  });

  it("does not reset when extraction throws", async () => {
    mocks.runTranscriptUtility.mockRejectedValue(new Error("network down"));
    const { result } = renderHook(() => useResumeWithHandoff(TASK_ID, SESSION_ID));

    await act(async () => {
      await result.current.run();
    });

    expect(mocks.requestContextReset).not.toHaveBeenCalled();
    expect(mocks.sendMessageRequest).not.toHaveBeenCalled();
    expect(result.current.error).toBe(RESUME_FAILED_ERROR);
  });

  it("does not send the handoff when reset fails", async () => {
    mocks.requestContextReset.mockRejectedValue(new Error(AGENT_BUSY));
    const { result } = renderHook(() => useResumeWithHandoff(TASK_ID, SESSION_ID));

    await act(async () => {
      await result.current.run();
    });

    expect(mocks.order).toEqual(["extract", "reset"]);
    expect(mocks.clearContextWindow).not.toHaveBeenCalled();
    expect(mocks.sendMessageRequest).not.toHaveBeenCalled();
    expect(result.current.error).toBe(RESUME_FAILED_ERROR);
  });

  it("re-runs the whole flow on a retry instead of reusing earlier state", async () => {
    mocks.sendMessageRequest.mockRejectedValueOnce(new Error("send failed"));
    const { result } = renderHook(() => useResumeWithHandoff(TASK_ID, SESSION_ID));

    await act(async () => {
      await result.current.run();
    });
    expect(result.current.error).toBe(HANDOFF_NOT_SENT_ERROR);

    mocks.order.length = 0;
    mocks.sendMessageRequest.mockResolvedValue({ id: "msg-1", session_id: SESSION_ID });
    await act(async () => {
      await result.current.run();
    });

    expect(mocks.order).toEqual(["extract", "reset", "send"]);
    expect(mocks.runTranscriptUtility).toHaveBeenCalledTimes(2);
    expect(mocks.requestContextReset).toHaveBeenCalledTimes(2);
    expect(result.current.error).toBeNull();
  });

  it("clears a previous session's error when the session changes", async () => {
    mocks.requestContextReset.mockRejectedValue(new Error(AGENT_BUSY));
    const { result, rerender } = renderHook(
      ({ sessionId }: { sessionId: string }) => useResumeWithHandoff(TASK_ID, sessionId),
      { initialProps: { sessionId: SESSION_ID } },
    );

    await act(async () => {
      await result.current.run();
    });
    expect(result.current.error).toBe(RESUME_FAILED_ERROR);

    rerender({ sessionId: "session-2" });
    expect(result.current.error).toBeNull();
  });

  it("hands the handoff to the caller when the send fails after a reset", async () => {
    mocks.sendMessageRequest.mockRejectedValueOnce(new Error("send failed"));
    const onHandoffNotSent = vi.fn();
    const { result } = renderHook(() =>
      useResumeWithHandoff(TASK_ID, SESSION_ID, { onHandoffNotSent }),
    );

    await act(async () => {
      await result.current.run();
    });

    expect(onHandoffNotSent).toHaveBeenCalledWith(HANDOFF_TEXT);
    expect(result.current.error).toBe(HANDOFF_NOT_SENT_ERROR);
  });

  it("does not hand off when the reset itself fails", async () => {
    mocks.requestContextReset.mockRejectedValue(new Error(AGENT_BUSY));
    const onHandoffNotSent = vi.fn();
    const { result } = renderHook(() =>
      useResumeWithHandoff(TASK_ID, SESSION_ID, { onHandoffNotSent }),
    );

    await act(async () => {
      await result.current.run();
    });

    expect(onHandoffNotSent).not.toHaveBeenCalled();
    expect(result.current.error).toBe(RESUME_FAILED_ERROR);
  });

  it("clears the error on demand so a dismissal can hide it", async () => {
    mocks.requestContextReset.mockRejectedValue(new Error(AGENT_BUSY));
    const { result } = renderHook(() => useResumeWithHandoff(TASK_ID, SESSION_ID));

    await act(async () => {
      await result.current.run();
    });
    expect(result.current.error).toBe(RESUME_FAILED_ERROR);

    act(() => result.current.clearError());
    expect(result.current.error).toBeNull();
  });

  it("translates an unavailable connection instead of surfacing the internal message", async () => {
    mocks.requestContextReset.mockRejectedValue(new WebSocketUnavailableError());
    const { result } = renderHook(() => useResumeWithHandoff(TASK_ID, SESSION_ID));

    await act(async () => {
      await result.current.run();
    });

    expect(result.current.error).toBe(RESUME_FAILED_ERROR);
  });
});
