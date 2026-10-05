import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { QuickChatSetup } from "./quick-chat-setup";

let defaultAgentId = "";
let promptValue = "";
let agentProfiles: Array<{ id: string; enabled?: boolean }> = [
  { id: "agent-a" },
  { id: "agent-b" },
];
const AGENT_SELECTOR_TEST_ID = "agent-profile-selector";

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      features: { dynamicAgentRouting: true },
      agentProfiles: { items: agentProfiles },
      userSettings: { defaultUtilityAgentProfileId: null },
      workspaces: {
        items: [{ id: "workspace-1", default_agent_profile_id: defaultAgentId }],
      },
    }),
}));

vi.mock("@/components/task-create-dialog-options", () => ({
  useAgentProfileOptions: () => [],
}));

vi.mock("@/components/task-create-dialog-selectors", () => ({
  AgentSelector: ({
    value,
    onValueChange,
    triggerClassName,
  }: {
    value: string;
    onValueChange: (id: string) => void;
    triggerClassName?: string;
  }) => (
    <button
      type="button"
      data-testid={AGENT_SELECTOR_TEST_ID}
      className={triggerClassName}
      onClick={() => onValueChange("agent-b")}
    >
      {value || "Select agent"}
    </button>
  ),
  TaskFormInputs: ({ descriptionValueRef }: { descriptionValueRef: { current: unknown } }) => {
    descriptionValueRef.current = {
      getValue: () => promptValue,
      setValue: () => {},
      getAttachments: () => [],
    };
    return <textarea data-testid="quick-chat-initial-prompt" />;
  },
}));

vi.mock("@/components/task-create-dialog-workspace-repo-chips", () => ({
  WorkspaceRepoChips: () => null,
}));

vi.mock("@kandev/ui/tooltip", () => ({
  Tooltip: ({ children }: { children: ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children: ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children: ReactNode }) => <>{children}</>,
}));

vi.mock("@/components/task-create-dialog-repositories-state", () => ({
  useRepositoriesState: () => ({
    repositories: [],
    addRepository: vi.fn(),
    removeRepository: vi.fn(),
    updateRepository: vi.fn(),
  }),
}));

vi.mock("@/hooks/domains/workspace/use-repositories", () => ({
  useRepositories: () => ({ repositories: [], isLoading: false }),
}));

const props = {
  workspaceId: "workspace-1",
  canCreateConfigurationChat: true,
  pendingAgentId: null,
  onStart: vi.fn(),
  onCancel: vi.fn(),
  onKindChange: vi.fn(),
};

beforeEach(() => {
  defaultAgentId = "";
  promptValue = "";
  agentProfiles = [{ id: "agent-a" }, { id: "agent-b" }];
  vi.clearAllMocks();
});

afterEach(cleanup);

describe("QuickChatSetup default agent", () => {
  it("explains when to use Quick Chat instead of a task", () => {
    render(<QuickChatSetup {...props} />);

    expect(screen.getByText(/idea, question, or codebase/i)).toBeTruthy();
    expect(screen.getByText(/outside your task board/i)).toBeTruthy();
  });

  it("offers configuration mode in the setup panel", () => {
    render(<QuickChatSetup {...props} />);

    fireEvent.click(screen.getByRole("switch", { name: "Configuration chat" }));

    expect(props.onKindChange).toHaveBeenCalledWith("config");
  });

  it("hides configuration mode when the workspace already has one", () => {
    render(<QuickChatSetup {...props} canCreateConfigurationChat={false} />);

    expect(screen.queryByRole("switch", { name: "Configuration chat" })).toBeNull();
  });

  it("renders the agent selector with a visible field border", () => {
    render(<QuickChatSetup {...props} />);

    expect(screen.getByTestId(AGENT_SELECTOR_TEST_ID).className).toContain("border-input");
  });

  it("uses a default agent that arrives after the setup mounts", () => {
    const { rerender } = render(<QuickChatSetup {...props} />);
    expect(screen.getByTestId(AGENT_SELECTOR_TEST_ID).textContent).toContain("Select agent");

    defaultAgentId = "agent-a";
    rerender(<QuickChatSetup {...props} />);

    expect(screen.getByTestId(AGENT_SELECTOR_TEST_ID).textContent).toContain("agent-a");
  });

  it("preserves an explicit selection when the workspace default changes", () => {
    const { rerender } = render(<QuickChatSetup {...props} />);
    fireEvent.click(screen.getByTestId(AGENT_SELECTOR_TEST_ID));
    expect(screen.getByTestId(AGENT_SELECTOR_TEST_ID).textContent).toContain("agent-b");

    defaultAgentId = "agent-a";
    rerender(<QuickChatSetup {...props} />);

    expect(screen.getByTestId(AGENT_SELECTOR_TEST_ID).textContent).toContain("agent-b");
  });

  it("clears a selected profile when it becomes disabled", () => {
    const { rerender } = render(<QuickChatSetup {...props} />);
    fireEvent.click(screen.getByTestId(AGENT_SELECTOR_TEST_ID));
    expect(screen.getByTestId(AGENT_SELECTOR_TEST_ID).textContent).toContain("agent-b");

    agentProfiles = [{ id: "agent-a" }, { id: "agent-b", enabled: false }];
    rerender(<QuickChatSetup {...props} />);

    expect(screen.getByTestId(AGENT_SELECTOR_TEST_ID).textContent).toContain("Select agent");
    expect((screen.getByTestId("quick-chat-start") as HTMLButtonElement).disabled).toBe(true);
    expect(props.onStart).not.toHaveBeenCalled();
  });

  it("submits the optional initial prompt with the start action", () => {
    defaultAgentId = "agent-a";
    promptValue = "Investigate the flaky test";
    render(<QuickChatSetup {...props} />);

    fireEvent.click(screen.getByTestId("quick-chat-start"));

    expect(props.onStart).toHaveBeenCalledWith("agent-a", [], "Investigate the flaky test");
  });

  it("starts without a prompt when the prompt is empty", () => {
    defaultAgentId = "agent-a";
    promptValue = "";
    render(<QuickChatSetup {...props} />);

    fireEvent.click(screen.getByTestId("quick-chat-start"));

    expect(props.onStart).toHaveBeenCalledWith("agent-a", [], undefined);
  });
});
