export type ProviderUsageRange = "24h" | "7d" | "30d";

export type ProviderUsageStatus =
  | "exhausted"
  | "nearing"
  | "stale"
  | "healthy"
  | "unknown"
  | "unavailable";

export type ProviderUsageTrend = "rising" | "flat" | "falling";
export type ProviderUsageConfidence = "none" | "low" | "medium";
export type ProviderUsageHistoryKind = "measured" | "estimated" | "limit_hit";

/** Closed set explaining why an account exposes no live quota source. */
export type ProviderUsageQuotaUnavailableReason =
  | "api_key_billing"
  | "credentials_missing"
  | "credentials_expired"
  | "no_provider_endpoint"
  | "profile_missing";

/** A prepaid amount left on the account. Informational; never a quota window. */
export type ProviderUsageBalance = {
  label: string;
  amount: number;
  unit: string;
};

export type ProviderUsageSubscriptionStatus =
  | "active"
  | "canceling"
  | "renewal_pending"
  | "inactive";

/** The provider-reported plan state. Informational only. */
export type ProviderUsageSubscription = {
  status: ProviderUsageSubscriptionStatus;
  plan?: string;
  period_ends_at?: string;
  uses_balance: boolean;
};

/** Closed set of provider credentials Kandev can store for live quota. */
export type ProviderUsageCredentialKind = "opencode_console_cookie" | "junie_api_key";

/** Describes the global secret that enables an account's live quota. */
export type ProviderUsageCredentialHint = {
  kind: ProviderUsageCredentialKind;
  agent_type: string;
  secret_name: string;
  configured: boolean;
};

export type ProviderUsageEstimate = {
  burn_rate_pct_per_hour: number;
  projected_exhaustion_at?: string;
  exhausts_before_reset: boolean;
  trend: ProviderUsageTrend;
  anomaly: boolean;
  confidence: ProviderUsageConfidence;
  source: "estimated";
  reset_at?: string;
};

export type ProviderUsageHistoryPoint = {
  at: string;
  pct: number;
  kind: ProviderUsageHistoryKind;
};

export type ProviderUsageWindow = {
  label: string;
  utilization_pct: number;
  reset_at?: string;
  source: "measured" | "estimated";
  estimate?: ProviderUsageEstimate;
  history: ProviderUsageHistoryPoint[];
};

export type ProviderUsageProfileRef = {
  id: string;
  name: string;
};

export type ProviderUsageAccount = {
  account_key: string;
  label?: string;
  plan?: string;
  status: ProviderUsageStatus;
  quota_unavailable_reason?: ProviderUsageQuotaUnavailableReason;
  profiles: ProviderUsageProfileRef[];
  windows: ProviderUsageWindow[];
  balances?: ProviderUsageBalance[];
  subscription?: ProviderUsageSubscription;
  credential?: ProviderUsageCredentialHint;
};

export type ProviderUsageProvider = {
  provider: string;
  display_name?: string;
  status: ProviderUsageStatus;
  accounts: ProviderUsageAccount[];
};

export type ProviderUsageSource = {
  source: string;
  enabled: boolean;
  toggleable: boolean;
};

export type ProviderUsageIndexState = "never" | "running" | "done" | "failed";

export type ProviderUsageIndexStatus = {
  state: ProviderUsageIndexState;
  files_total: number;
  files_done: number;
  started_at?: string;
  finished_at?: string;
  error_code?: string;
};

export type ProviderUsageOverview = {
  range: ProviderUsageRange;
  truncated: boolean;
  indexing: ProviderUsageIndexStatus;
  sources: ProviderUsageSource[];
  providers: ProviderUsageProvider[];
};

/** Breakdown grouping dimensions, mirroring the backend whitelist. */
export type UsageBreakdownGroupBy = "session" | "task" | "model" | "provider" | "agent" | "day";

export type UsageBreakdownSort = "tokens" | "cost" | "events" | "last_at";
export type UsageBreakdownOrder = "asc" | "desc";

/**
 * Breakdown query. `taskLabel` and `sessionLabel` are display-only: they label
 * trail-off filter chips for id-valued filters and are never sent to the API.
 */
export type UsageBreakdownQuery = {
  range?: ProviderUsageRange;
  groupBy?: UsageBreakdownGroupBy;
  provider?: string;
  model?: string;
  agentType?: string;
  taskId?: string;
  taskLabel?: string;
  sessionId?: string;
  sessionLabel?: string;
  q?: string;
  sort?: UsageBreakdownSort;
  order?: UsageBreakdownOrder;
  limit?: number;
  offset?: number;
};

export type UsageBreakdownRow = {
  key: string;
  label: string;
  task_id: string;
  session_name: string;
  provider: string;
  model: string;
  models: number;
  tokens_total: number;
  tokens_in: number;
  tokens_out: number;
  tokens_cached_read: number;
  cost_subcents: number;
  events: number;
  first_at: string;
  last_at: string;
};

export type UsageBreakdownFacets = {
  providers: string[];
  models: string[];
  agent_types: string[];
};

export type UsageBreakdownResponse = {
  range: ProviderUsageRange;
  group_by: UsageBreakdownGroupBy;
  total_rows: number;
  total_tokens: number;
  total_cost_subcents: number;
  total_events: number;
  facets: UsageBreakdownFacets;
  rows: UsageBreakdownRow[];
};
