import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";

const mockStartQuickChat = vi.fn();
const mockDeleteTask = vi.fn();
const recordRecentUseMock = vi.fn();

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: vi.fn() }),
}));

vi.mock("@/lib/api/domains/workspace-api", () => ({
  startQuickChat: (...args: unknown[]) => mockStartQuickChat(...args),
}));

vi.mock("@/lib/api/domains/kanban-api", () => ({
  deleteTask: (...args: unknown[]) => mockDeleteTask(...args),
}));

vi.mock("@/lib/agent-profile-recent-use", () => ({
  recordAgentProfileRecentUseBestEffort: (...args: unknown[]) => recordRecentUseMock(...args),
}));

import { useAgentSelection } from "./use-quick-chat-modal";

const WORKSPACE_ID = "ws-1";
const AGENT_ID = "agent-a";
const PROMPT = "Investigate the flaky test";

function makeStore(cliPassthrough: boolean) {
  return {
    agentProfiles: [
      {
        id: AGENT_ID,
        label: "Agent A",
        agent_id: "a",
        agent_name: "Agent A",
        cli_passthrough: cliPassthrough,
      },
    ],
    sessions: [],
    activeSessionId: "",
    agentGeneratedTaskTitles: false,
    openQuickChat: vi.fn(),
    renameQuickChatSession: vi.fn(),
    setQuickChatInitialPrompt: vi.fn(),
    closeQuickChatSession: vi.fn(),
    applyAgentProfileRecentUse: vi.fn(),
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  mockStartQuickChat.mockResolvedValue({ task_id: "task-a", session_id: "sess-a" });
});

describe("useAgentSelection initial prompt", () => {
  it("submits a provided prompt through the shared initial-prompt path for ACP agents", async () => {
    const store = makeStore(false);
    const { result } = renderHook(() => useAgentSelection(WORKSPACE_ID, store as never));

    await act(async () => {
      await result.current.handleSelectAgent(AGENT_ID, [], PROMPT);
    });

    expect(store.setQuickChatInitialPrompt).toHaveBeenCalledWith("sess-a", PROMPT);
    expect(mockStartQuickChat).toHaveBeenCalledWith(
      WORKSPACE_ID,
      expect.not.objectContaining({ prompt: expect.any(String) }),
    );
    expect(store.renameQuickChatSession).toHaveBeenCalledWith("sess-a", PROMPT);
  });

  it("sends the prompt in the start request for passthrough agents", async () => {
    const store = makeStore(true);
    const { result } = renderHook(() => useAgentSelection(WORKSPACE_ID, store as never));

    await act(async () => {
      await result.current.handleSelectAgent(AGENT_ID, [], PROMPT);
    });

    expect(mockStartQuickChat).toHaveBeenCalledWith(
      WORKSPACE_ID,
      expect.objectContaining({ prompt: PROMPT }),
    );
    expect(store.setQuickChatInitialPrompt).not.toHaveBeenCalled();
  });
});
