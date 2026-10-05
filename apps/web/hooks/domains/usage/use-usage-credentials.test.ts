import { describe, expect, it } from "vitest";
import { credentialForAgent } from "./use-usage-credentials";
import type { ProviderUsageCredentialHint } from "@/lib/types/provider-usage";

const hints: ProviderUsageCredentialHint[] = [
  {
    kind: "opencode_console_cookie",
    agent_type: "opencode-acp",
    secret_name: "opencode-console-cookie",
    configured: false,
  },
  {
    kind: "junie_api_key",
    agent_type: "junie-acp",
    secret_name: "junie-api-key",
    configured: true,
  },
];

describe("credentialForAgent", () => {
  it("finds the descriptor whose agent type matches", () => {
    expect(credentialForAgent(hints, "junie-acp")?.secret_name).toBe("junie-api-key");
  });

  it("returns null for an agent type without a descriptor", () => {
    expect(credentialForAgent(hints, "claude-acp")).toBeNull();
  });

  it("returns null for an empty list", () => {
    expect(credentialForAgent([], "junie-acp")).toBeNull();
  });
});
