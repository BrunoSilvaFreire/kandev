"use client";

import { useCallback, useEffect, useState } from "react";
import { fetchUsageCredentials } from "@/lib/api/domains/provider-usage-api";
import type { ProviderUsageCredentialHint } from "@/lib/types/provider-usage";

/** Find the credential descriptor for an agent type, or null when unsupported. */
export function credentialForAgent(
  hints: ProviderUsageCredentialHint[],
  agentType: string,
): ProviderUsageCredentialHint | null {
  return hints.find((hint) => hint.agent_type === agentType) ?? null;
}

/**
 * Load the provider-credential descriptors and resolve the one for a given
 * agent type. The descriptors are the same for every profile.
 */
export function useUsageCredentials(agentType: string | undefined) {
  const [hints, setHints] = useState<ProviderUsageCredentialHint[]>([]);

  const load = useCallback(async () => {
    try {
      setHints(await fetchUsageCredentials());
    } catch {
      setHints([]);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const hint = agentType ? credentialForAgent(hints, agentType) : null;
  return { hint, refresh: load };
}
