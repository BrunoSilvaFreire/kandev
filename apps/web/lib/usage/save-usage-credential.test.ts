import { beforeEach, describe, expect, it, vi } from "vitest";
import { createSecret, listSecrets, updateSecret } from "@/lib/api/domains/secrets-api";
import { refreshUsageCredentials } from "@/lib/api/domains/provider-usage-api";
import { saveUsageCredential } from "./save-usage-credential";
import type { ProviderUsageCredentialHint } from "@/lib/types/provider-usage";

vi.mock("@/lib/api/domains/secrets-api", () => ({
  createSecret: vi.fn(),
  updateSecret: vi.fn(),
  listSecrets: vi.fn(),
}));
vi.mock("@/lib/api/domains/provider-usage-api", () => ({
  refreshUsageCredentials: vi.fn(),
}));

const hint: ProviderUsageCredentialHint = {
  kind: "junie_api_key",
  agent_type: "junie-acp",
  secret_name: "junie-api-key",
  configured: false,
};

describe("saveUsageCredential", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("creates a global secret when none exists", async () => {
    vi.mocked(listSecrets).mockResolvedValue([]);
    await saveUsageCredential(hint, "  key-value  ");
    expect(createSecret).toHaveBeenCalledWith({
      name: "junie-api-key",
      value: "key-value",
      scope: "global",
    });
    expect(updateSecret).not.toHaveBeenCalled();
    expect(refreshUsageCredentials).toHaveBeenCalledTimes(1);
  });

  it("updates the existing secret", async () => {
    vi.mocked(listSecrets).mockResolvedValue([
      {
        id: "secret-1",
        name: "junie-api-key",
        scope: "global",
        workspace_id: "",
        has_value: true,
        created_at: "",
        updated_at: "",
      },
    ]);
    await saveUsageCredential(hint, "new-value");
    expect(updateSecret).toHaveBeenCalledWith("secret-1", { value: "new-value" });
    expect(createSecret).not.toHaveBeenCalled();
  });

  it("rejects an empty value without calling the API", async () => {
    await expect(saveUsageCredential(hint, "   ")).rejects.toThrow();
    expect(listSecrets).not.toHaveBeenCalled();
    expect(createSecret).not.toHaveBeenCalled();
  });

  it("propagates a create failure", async () => {
    vi.mocked(listSecrets).mockResolvedValue([]);
    vi.mocked(createSecret).mockRejectedValue(new Error("boom"));
    await expect(saveUsageCredential(hint, "value")).rejects.toThrow("boom");
    expect(refreshUsageCredentials).not.toHaveBeenCalled();
  });
});
