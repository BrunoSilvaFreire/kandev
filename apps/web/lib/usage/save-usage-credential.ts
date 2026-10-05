import { createSecret, listSecrets, updateSecret } from "@/lib/api/domains/secrets-api";
import { refreshUsageCredentials } from "@/lib/api/domains/provider-usage-api";
import type { ProviderUsageCredentialHint } from "@/lib/types/provider-usage";

/**
 * Save a provider credential as the global secret the hint names. An existing
 * secret with the same name is updated in place; otherwise one is created. The
 * live quota cache is dropped afterward so the value takes effect on the next
 * read. The value is never echoed back to the caller.
 */
export async function saveUsageCredential(
  hint: ProviderUsageCredentialHint,
  value: string,
): Promise<void> {
  const trimmed = value.trim();
  if (!trimmed) throw new Error("credential value is empty");
  const items = await listSecrets({ scope: "global" });
  const existing = items.find((item) => item.name === hint.secret_name);
  if (existing) {
    await updateSecret(existing.id, { value: trimmed });
  } else {
    await createSecret({ name: hint.secret_name, value: trimmed, scope: "global" });
  }
  await refreshUsageCredentials();
}
