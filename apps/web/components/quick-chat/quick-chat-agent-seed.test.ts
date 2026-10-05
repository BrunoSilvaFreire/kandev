import { describe, expect, it } from "vitest";
import type { AgentProfileOption } from "@/lib/state/slices";
import { seedQuickChatAgentProfileId } from "./quick-chat-agent-seed";

function profile(id: string, overrides: Partial<AgentProfileOption> = {}): AgentProfileOption {
  return {
    id,
    label: id,
    agent_id: id,
    agent_name: id,
    cli_passthrough: false,
    ...overrides,
  };
}

describe("seedQuickChatAgentProfileId", () => {
  it("prefers the selectable utility agent profile", () => {
    const result = seedQuickChatAgentProfileId({
      agentProfiles: [profile("utility"), profile("workspace")],
      utilityAgentProfileId: "utility",
      workspaceDefaultAgentProfileId: "workspace",
      dynamicRoutingEnabled: true,
    });

    expect(result).toBe("utility");
  });

  it("falls back to the workspace default when the utility profile is absent", () => {
    const result = seedQuickChatAgentProfileId({
      agentProfiles: [profile("workspace")],
      utilityAgentProfileId: "missing",
      workspaceDefaultAgentProfileId: "workspace",
      dynamicRoutingEnabled: true,
    });

    expect(result).toBe("workspace");
  });

  it("falls back when the utility profile is disabled", () => {
    const result = seedQuickChatAgentProfileId({
      agentProfiles: [profile("utility", { enabled: false }), profile("workspace")],
      utilityAgentProfileId: "utility",
      workspaceDefaultAgentProfileId: "workspace",
      dynamicRoutingEnabled: true,
    });

    expect(result).toBe("workspace");
  });

  it("returns an empty string when neither profile is selectable", () => {
    const result = seedQuickChatAgentProfileId({
      agentProfiles: [profile("dynamic", { kind: "dynamic" })],
      utilityAgentProfileId: "dynamic",
      workspaceDefaultAgentProfileId: "dynamic",
      dynamicRoutingEnabled: false,
    });

    expect(result).toBe("");
  });
});
