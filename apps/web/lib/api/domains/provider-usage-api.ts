import { fetchJson, type ApiRequestOptions } from "../client";
import type {
  ProviderUsageCredentialHint,
  ProviderUsageIndexStatus,
  ProviderUsageOverview,
  ProviderUsageRange,
  ProviderUsageSource,
  UsageBreakdownQuery,
  UsageBreakdownResponse,
} from "@/lib/types/provider-usage";

const PROVIDER_USAGE_PATH = "/api/v1/provider-usage";
const PROVIDER_USAGE_BREAKDOWN_PATH = "/api/v1/provider-usage/breakdown";
const PROVIDER_USAGE_INDEX_PATH = "/api/v1/provider-usage/index";
const PROVIDER_USAGE_SOURCES_PATH = "/api/v1/provider-usage/sources";
const PROVIDER_USAGE_CREDENTIALS_PATH = "/api/v1/provider-usage/credentials";
const PROVIDER_USAGE_CREDENTIALS_REFRESH_PATH = "/api/v1/provider-usage/credentials/refresh";

/** Read the credential descriptors the usage dialog and settings card need. */
export async function fetchUsageCredentials(
  options?: ApiRequestOptions,
): Promise<ProviderUsageCredentialHint[]> {
  return fetchJson<ProviderUsageCredentialHint[]>(PROVIDER_USAGE_CREDENTIALS_PATH, options);
}

/** Drop cached live quota so a freshly saved credential is reflected. */
export async function refreshUsageCredentials(options?: ApiRequestOptions): Promise<void> {
  await fetchJson<{ refreshed: boolean }>(PROVIDER_USAGE_CREDENTIALS_REFRESH_PATH, {
    ...options,
    init: { method: "POST", ...(options?.init ?? {}) },
  });
}

/** Read the provider usage overview for one range. */
export async function fetchProviderUsage(
  range: ProviderUsageRange,
  options?: ApiRequestOptions,
): Promise<ProviderUsageOverview> {
  return fetchJson<ProviderUsageOverview>(`${PROVIDER_USAGE_PATH}?range=${range}`, options);
}

/**
 * Read the grouped ledger breakdown. Empty values are omitted; the backend
 * applies its own defaults and clamps pagination.
 */
export async function fetchUsageBreakdown(
  query: UsageBreakdownQuery,
  options?: ApiRequestOptions,
): Promise<UsageBreakdownResponse> {
  const params = new URLSearchParams();
  const entries: Array<[string, string | number | undefined]> = [
    ["range", query.range],
    ["group_by", query.groupBy],
    ["provider", query.provider],
    ["model", query.model],
    ["agent_type", query.agentType],
    ["task_id", query.taskId],
    ["session_id", query.sessionId],
    ["q", query.q],
    ["sort", query.sort],
    ["order", query.order],
    ["limit", query.limit],
    ["offset", query.offset],
  ];
  for (const [key, value] of entries) {
    if (value !== undefined && value !== "") params.set(key, String(value));
  }
  const search = params.toString();
  const suffix = search ? `?${search}` : "";
  return fetchJson<UsageBreakdownResponse>(`${PROVIDER_USAGE_BREAKDOWN_PATH}${suffix}`, options);
}

/** Read the history index job's status. */
export async function fetchUsageIndexStatus(
  options?: ApiRequestOptions,
): Promise<ProviderUsageIndexStatus> {
  return fetchJson<ProviderUsageIndexStatus>(PROVIDER_USAGE_INDEX_PATH, options);
}

/** Start (or join) the single-flight history index job. */
export async function startUsageIndex(
  options?: ApiRequestOptions,
): Promise<ProviderUsageIndexStatus> {
  return fetchJson<ProviderUsageIndexStatus>(PROVIDER_USAGE_INDEX_PATH, {
    ...options,
    init: { method: "POST", ...(options?.init ?? {}) },
  });
}

/** Change one or more opt-in history sources. */
export async function updateUsageSources(
  sources: Partial<Record<"claude_local" | "codex_local" | "antigravity_local", boolean>>,
  options?: ApiRequestOptions,
): Promise<ProviderUsageSource[]> {
  const response = await fetchJson<{ sources: ProviderUsageSource[] }>(
    PROVIDER_USAGE_SOURCES_PATH,
    {
      ...options,
      init: {
        method: "PUT",
        body: JSON.stringify(sources),
        ...(options?.init ?? {}),
      },
    },
  );
  return response.sources ?? [];
}
