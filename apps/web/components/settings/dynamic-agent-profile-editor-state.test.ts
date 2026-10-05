import { describe, expect, it } from "vitest";
import type { DynamicAgentCandidate, DynamicErrorPolicy } from "@/lib/types/agent-profile";
import { agentProfileId } from "@/lib/types/ids";
import { dynamicProfilePayload, dynamicTagConflict } from "./dynamic-agent-profile-editor-state";

function policy(): DynamicErrorPolicy {
  return {
    retry: { enabled: false, maxRetries: 0, initialIntervalSeconds: 0 },
    waitForReset: { enabled: false, maxWaitSeconds: 0 },
    onExhausted: "skip",
  };
}

const candidate: DynamicAgentCandidate = {
  position: 0,
  executionProfileId: agentProfileId("candidate"),
  enabled: true,
  policies: {
    version: 1,
    transient: policy(),
    hard: policy(),
    unclassified: { enabled: true, consecutiveFailureThreshold: 4 },
  },
};

describe("dynamicTagConflict", () => {
  it("reports a tag that is both preferred and avoided", () => {
    expect(dynamicTagConflict(["claude"], ["gemini"])).toBeNull();
    expect(dynamicTagConflict(["claude"], ["claude"])).toBe("claude");
  });
});

describe("dynamicProfilePayload", () => {
  it("emits snake_case preference lists and ordered candidates", () => {
    const payload = dynamicProfilePayload({
      name: " Review ",
      enabled: true,
      version: 2,
      preferredTags: ["claude"],
      avoidedTags: ["gemini"],
      candidates: [candidate],
    });
    expect(payload.name).toBe("Review");
    expect(payload.dynamic.preferred_tags).toEqual(["claude"]);
    expect(payload.dynamic.avoided_tags).toEqual(["gemini"]);
    expect(payload.dynamic.candidates).toHaveLength(1);
    expect(payload.dynamic.candidates[0].execution_profile_id).toBe("candidate");
  });
});
