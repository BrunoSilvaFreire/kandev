import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { runTranscriptUtility, useSummarizeSession } from "./use-summarize-session";

const mockListMessages = vi.fn();
const SESSION_ID = "session-1";
const HANDOFF_AGENT_ID = "builtin-extract-resume-handoff";
const CONNECTION_REFUSED = "connection refused";
const mockExecuteUtilityPrompt = vi.fn();

vi.mock("@/lib/api/domains/session-api", () => ({
  listTaskSessionMessages: (...args: unknown[]) => mockListMessages(...args),
}));

vi.mock("@/lib/api/domains/utility-api", () => ({
  executeUtilityPrompt: (...args: unknown[]) => mockExecuteUtilityPrompt(...args),
}));

describe("useSummarizeSession", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("returns the generated summary", async () => {
    mockListMessages.mockResolvedValue({
      messages: [{ type: "message", author_type: "user", content: "hello" }],
    });
    mockExecuteUtilityPrompt.mockResolvedValue({ success: true, response: "summary" });
    const { result } = renderHook(() => useSummarizeSession());

    let summary;
    await act(async () => {
      summary = await result.current.summarize(SESSION_ID);
    });

    expect(summary).toEqual({ summary: "summary" });
  });

  it("returns backend execution errors instead of swallowing them", async () => {
    mockListMessages.mockResolvedValue({
      messages: [{ type: "message", author_type: "user", content: "hello" }],
    });
    mockExecuteUtilityPrompt.mockRejectedValue(new Error(CONNECTION_REFUSED));
    const { result } = renderHook(() => useSummarizeSession());

    let summary;
    await act(async () => {
      summary = await result.current.summarize(SESSION_ID);
    });

    expect(summary).toEqual({ summary: null, error: CONNECTION_REFUSED });
    expect(result.current.isSummarizing).toBe(false);
  });

  it("returns a plain empty result for a session with no messages", async () => {
    mockListMessages.mockResolvedValue({ messages: [] });
    const { result } = renderHook(() => useSummarizeSession());

    let summary;
    await act(async () => {
      summary = await result.current.summarize(SESSION_ID);
    });

    expect(summary).toEqual({ summary: null });
    expect(mockExecuteUtilityPrompt).not.toHaveBeenCalled();
  });
});

describe("runTranscriptUtility", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("reports an empty transcript without calling the utility", async () => {
    mockListMessages.mockResolvedValue({ messages: [] });

    const result = await runTranscriptUtility(SESSION_ID, HANDOFF_AGENT_ID);

    expect(result).toEqual({ status: "empty" });
    expect(mockExecuteUtilityPrompt).not.toHaveBeenCalled();
  });

  it("returns the utility response and passes the requested agent id", async () => {
    mockListMessages.mockResolvedValue({
      messages: [{ type: "message", author_type: "user", content: "hello" }],
    });
    mockExecuteUtilityPrompt.mockResolvedValue({ success: true, response: "handoff" });

    const result = await runTranscriptUtility(SESSION_ID, HANDOFF_AGENT_ID);

    expect(result).toEqual({ status: "ok", text: "handoff" });
    expect(mockExecuteUtilityPrompt).toHaveBeenCalledWith({
      utility_agent_id: HANDOFF_AGENT_ID,
      conversation_history: "User: hello",
    });
  });

  it("surfaces a utility-level failure with its error", async () => {
    mockListMessages.mockResolvedValue({
      messages: [{ type: "message", author_type: "user", content: "hello" }],
    });
    mockExecuteUtilityPrompt.mockResolvedValue({ success: false, error: "model unavailable" });

    const result = await runTranscriptUtility(SESSION_ID, HANDOFF_AGENT_ID);

    expect(result).toEqual({ status: "failed", error: "model unavailable" });
  });

  it("propagates transport failures to the caller", async () => {
    mockListMessages.mockRejectedValue(new Error(CONNECTION_REFUSED));

    await expect(runTranscriptUtility(SESSION_ID, HANDOFF_AGENT_ID)).rejects.toThrow(
      CONNECTION_REFUSED,
    );
  });
});
