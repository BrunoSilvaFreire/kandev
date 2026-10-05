import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { AgentProfile } from "@/lib/types/http";
import { agentProfileId } from "@/lib/types/ids";
import {
  buildDynamicDraftDocument,
  dynamicDraftRevision,
  useDynamicAgentProfileEditorDraft,
} from "./dynamic-agent-profile-editor-draft";

function makeProfile(): AgentProfile {
  return {
    id: agentProfileId("dynamic-1"),
    kind: "dynamic",
    name: "Review",
    agentId: "dynamic",
    agentDisplayName: "Dynamic",
    model: "",
    allowIndexing: false,
    autoApprove: false,
    cliFlags: [],
    cliPassthrough: false,
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
    dynamic: {
      version: 1,
      preferredTags: ["claude"],
      avoidedTags: ["gemini"],
      candidates: [],
    },
  } as AgentProfile;
}

describe("buildDynamicDraftDocument", () => {
  it("carries the preference lists", () => {
    const document = buildDynamicDraftDocument(3, ["claude"], ["gemini"], []);
    expect(document).toEqual({
      version: 3,
      preferredTags: ["claude"],
      avoidedTags: ["gemini"],
      candidates: [],
    });
  });
});

describe("dynamicDraftRevision", () => {
  it("changes when a preference tag changes", () => {
    const base = dynamicDraftRevision("Review", ["claude"], ["gemini"], [], true);
    expect(dynamicDraftRevision("Review", ["codex"], ["gemini"], [], true)).not.toBe(base);
    expect(dynamicDraftRevision("Review", ["claude"], [], [], true)).not.toBe(base);
    expect(dynamicDraftRevision("Review", ["claude"], ["gemini"], [], true)).toBe(base);
  });
});

describe("useDynamicAgentProfileEditorDraft preferences", () => {
  it("initializes from the profile and propagates updates", () => {
    const onDraftChange = vi.fn();
    const { result } = renderHook(() =>
      useDynamicAgentProfileEditorDraft({ profile: makeProfile(), onDraftChange }),
    );
    expect(result.current.preferredTags).toEqual(["claude"]);
    expect(result.current.avoidedTags).toEqual(["gemini"]);

    act(() => result.current.updatePreferredTags(["claude", "subscription"]));

    expect(result.current.preferredTags).toEqual(["claude", "subscription"]);
    expect(result.current.currentProfile.dynamic?.preferredTags).toEqual([
      "claude",
      "subscription",
    ]);
    expect(onDraftChange).toHaveBeenLastCalledWith(
      expect.objectContaining({
        dynamic: expect.objectContaining({
          preferredTags: ["claude", "subscription"],
          avoidedTags: ["gemini"],
        }),
      }),
    );
  });

  it("resets preferences to the saved profile", () => {
    const { result } = renderHook(() =>
      useDynamicAgentProfileEditorDraft({ profile: makeProfile() }),
    );
    act(() => result.current.updateAvoidedTags(["codex"]));
    expect(result.current.avoidedTags).toEqual(["codex"]);
    act(() => result.current.reset());
    expect(result.current.avoidedTags).toEqual(["gemini"]);
  });
});
