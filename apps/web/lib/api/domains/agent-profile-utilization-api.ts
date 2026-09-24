import { fetchJson, type ApiRequestOptions } from "../client";
import type { ProviderUsage } from "@/lib/types/agent-profile";

export type ProfileUtilizationState = "known" | "unknown" | "unavailable";

export type ProfileUtilizationItem = {
  profile_id: string;
  state: ProfileUtilizationState;
  remaining_pct?: number;
  utilization?: ProviderUsage;
};

type ProfileUtilizationResponse = {
  profiles: ProfileUtilizationItem[];
};

const PROFILE_UTILIZATION_PATH = "/api/v1/agent-profiles/utilization";

/** Batch subscription utilization for the requested concrete profile IDs. */
export async function fetchProfileUtilization(
  profileIds: string[],
  options?: ApiRequestOptions,
): Promise<ProfileUtilizationItem[]> {
  const response = await fetchJson<ProfileUtilizationResponse>(PROFILE_UTILIZATION_PATH, {
    ...options,
    init: {
      method: "POST",
      body: JSON.stringify({ profile_ids: profileIds }),
      ...(options?.init ?? {}),
    },
  });
  return response.profiles ?? [];
}
