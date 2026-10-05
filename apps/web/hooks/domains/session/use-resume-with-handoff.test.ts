import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";
import {
  CACHE_EXPIRY_MS,
  isResumeHandoffCandidate,
  useResumeWithHandoff,
} from "./use-resume-with-handoff";

const mocks = vi.hoisted(() => ({
  resumeWithHandoff: vi.fn(),
  clearContextWindow: vi.fn(),
}));

vi.mock("@/lib/api/domains/session-api", () => ({
  resumeWithHandoff: (...args: unknown[]) => mocks.resumeWithHandoff(...args),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (
    selector: (state: { clearContextWindow: typeof mocks.clearContextWindow }) => unknown,
  ) =>
    selector({
      clearContextWindow: mocks.clearContextWindow,
    }),
}));

vi.mock("@/lib/i18n", () => ({
  t: (key: string) => key,
}));

const TASK_ID = "task-1";
const SESSION_ID = "session-1";
const HANDOFF_TEXT = "handoff text";
const COMPOSED_PROMPT = "handoff text\n\n## Additional instructions\n\nkeep going";
const RESUME_FAILED_ERROR = "task:resumeHandoffFailed";
const EXTRACT_FAILED_ERROR = "task:resumeHandoffCouldNotExtract";
const HANDOFF_NOT_SENT_ERROR = "task:resumeHandoffNotSent";

function candidateInput(overrides: Partial<Parameters<typeof isResumeHandoffCandidate>[0]> = {}) {
  return {
    state: "WAITING_FOR_INPUT" as const,
    isAgentBusy: false,
    hasLiveExecution: true,
    hasAgentMessage: true,
    lastUsageEventAt: 1_000,
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
    ["there is no newest usage event", { lastUsageEventAt: null }],
  ])("rejects when %s", (_label, overrides) => {
    expect(isResumeHandoffCandidate(candidateInput(overrides))).toBe(false);
  });
});

describe("useResumeWithHandoff", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    mocks.resumeWithHandoff.mockResolvedValue({
      handoff: HANDOFF_TEXT,
      prompt: COMPOSED_PROMPT,
      sent: true,
    });
  });

  it("calls resumeWithHandoff, clears the context window, and returns true", async () => {
    const { result } = renderHook(() => useResumeWithHandoff(TASK_ID, SESSION_ID));

    let success: boolean | undefined;
    await act(async () => {
      success = await result.current.run("keep going");
    });

    expect(success).toBe(true);
    expect(mocks.resumeWithHandoff).toHaveBeenCalledWith(SESSION_ID, "keep going");
    expect(mocks.clearContextWindow).toHaveBeenCalledWith(SESSION_ID);
    expect(result.current.error).toBeNull();
    expect(result.current.isRunning).toBe(false);
  });

  it("hands off prompt to caller when sent is false", async () => {
    mocks.resumeWithHandoff.mockResolvedValue({
      handoff: HANDOFF_TEXT,
      prompt: COMPOSED_PROMPT,
      sent: false,
    });
    const onHandoffNotSent = vi.fn();
    const { result } = renderHook(() =>
      useResumeWithHandoff(TASK_ID, SESSION_ID, { onHandoffNotSent }),
    );

    let success: boolean | undefined;
    await act(async () => {
      success = await result.current.run();
    });

    expect(success).toBe(false);
    expect(mocks.clearContextWindow).not.toHaveBeenCalled();
    expect(onHandoffNotSent).toHaveBeenCalledWith(COMPOSED_PROMPT);
    expect(result.current.error).toBe(HANDOFF_NOT_SENT_ERROR);
  });

  it("sets extraction error when ApiError has errorCode extraction_failed", async () => {
    mocks.resumeWithHandoff.mockRejectedValue(
      new ApiError("extraction failed", 500, { error_code: "extraction_failed" }),
    );
    const { result } = renderHook(() => useResumeWithHandoff(TASK_ID, SESSION_ID));

    let success: boolean | undefined;
    await act(async () => {
      success = await result.current.run();
    });

    expect(success).toBe(false);
    expect(mocks.clearContextWindow).not.toHaveBeenCalled();
    expect(result.current.error).toBe(EXTRACT_FAILED_ERROR);
  });

  it("sets generic error when resume fails", async () => {
    mocks.resumeWithHandoff.mockRejectedValue(new Error("network error"));
    const { result } = renderHook(() => useResumeWithHandoff(TASK_ID, SESSION_ID));

    let success: boolean | undefined;
    await act(async () => {
      success = await result.current.run();
    });

    expect(success).toBe(false);
    expect(mocks.clearContextWindow).not.toHaveBeenCalled();
    expect(result.current.error).toBe(RESUME_FAILED_ERROR);
  });

  it("clears a previous session's error when the session changes", async () => {
    mocks.resumeWithHandoff.mockRejectedValue(new Error("failed"));
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

  it("clears the error on demand so a dismissal can hide it", async () => {
    mocks.resumeWithHandoff.mockRejectedValue(new Error("failed"));
    const { result } = renderHook(() => useResumeWithHandoff(TASK_ID, SESSION_ID));

    await act(async () => {
      await result.current.run();
    });
    expect(result.current.error).toBe(RESUME_FAILED_ERROR);

    act(() => result.current.clearError());
    expect(result.current.error).toBeNull();
  });

  it("does nothing and returns false when taskId or sessionId is missing", async () => {
    const { result } = renderHook(() => useResumeWithHandoff(null, null));

    let success: boolean | undefined;
    await act(async () => {
      success = await result.current.run();
    });

    expect(success).toBe(false);
    expect(mocks.resumeWithHandoff).not.toHaveBeenCalled();
  });
});
