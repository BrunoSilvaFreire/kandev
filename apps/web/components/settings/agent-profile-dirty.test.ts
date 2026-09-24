import { describe, expect, it } from "vitest";
import { isProfileDirty } from "./agent-profile-dirty";
import type { AgentProfile } from "@/lib/types/http";

function profile(overrides: Partial<AgentProfile>): AgentProfile {
  return {
    id: "profile-1",
    name: "Reviewer",
    agentId: "claude-acp",
    agentDisplayName: "Claude",
    model: "sonnet",
    allowIndexing: false,
    autoApprove: false,
    cliFlags: [],
    cliPassthrough: false,
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
    ...overrides,
  } as AgentProfile;
}

describe("isProfileDirty with tags", () => {
  it("detects added and removed tags", () => {
    expect(isProfileDirty(profile({ tags: ["review"] }), profile({ tags: [] }), {})).toBe(true);
    expect(isProfileDirty(profile({ tags: [] }), profile({ tags: ["review"] }), {})).toBe(true);
  });

  it("treats an equal canonical list as clean", () => {
    expect(
      isProfileDirty(profile({ tags: ["review", "security"] }), profile({ tags: ["review", "security"] }), {}),
    ).toBe(false);
  });
});
