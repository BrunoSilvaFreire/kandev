---
status: current
system: costs
requirements:
  - REQ-COSTS-PROVIDER-USAGE-001
  - REQ-COSTS-PROVIDER-USAGE-002
  - REQ-COSTS-PROVIDER-USAGE-003
  - REQ-COSTS-PROVIDER-USAGE-004
  - REQ-COSTS-PROVIDER-USAGE-005
  - REQ-COSTS-PROVIDER-USAGE-006
---

# Provider Usage Monitoring System Design

## Purpose and boundaries

This design owns one durable provider-usage model: observations, sources,
backfill cursors, estimation, and the `/usage` HTTP and web surface. It builds
on the existing live fetch path and does not replace it.

The live clients stay in `internal/agent/usage` (`UsageService`, `UsageCache`,
`ProviderUsageClient`). The structured cache adapter in
`internal/backendapp/usage_adapter.go` remains the only registration point and
gains an Antigravity case plus a `CacheKeyFor` lookup. The new package
`internal/providerusage` owns storage, backfill, estimation, and HTTP.

The page is not part of Office. It is a top-level SPA route and a sidebar
destination.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-COSTS-PROVIDER-USAGE-001` | [Placement and routing](#placement-and-routing), [HTTP surface](#http-surface), [Frontend](#frontend) |
| `REQ-COSTS-PROVIDER-USAGE-002` | [Estimation](#estimation), [Persistence](#persistence) |
| `REQ-COSTS-PROVIDER-USAGE-003` | [Sources and consent](#sources-and-consent), [Backfill](#backfill) |
| `REQ-COSTS-PROVIDER-USAGE-004` | [Antigravity client](#antigravity-client), [Live and limit-hit recording](#live-and-limit-hit-recording) |
| `REQ-COSTS-PROVIDER-USAGE-005` | [Identity and attribution](#identity-and-attribution), [HTTP surface](#http-surface) |
| `REQ-COSTS-PROVIDER-USAGE-006` | [Gemini Code Assist client](#gemini-code-assist-client), [Model-scoped windows](#model-scoped-windows), [Identity and attribution](#identity-and-attribution) |
| `REQ-COSTS-PROVIDER-USAGE-007` | [Live and limit-hit recording](#live-and-limit-hit-recording), [OpenCode Go client](#opencode-go-client) |

## Components and responsibilities

- `internal/agent/usage`: owns live clients, the cache, and the new optional
  fetch recorder. `UsageCache.SetOnFetched` and `UsageService.SetRecorder` fire
  only after a real, successful, non-nil fetch. `UsageService.CacheKeyFor`
  exposes the cache key registered for a profile.
- `internal/agent/usage/client_antigravity.go`: discovers the running
  Antigravity language server, calls it over loopback, and maps its quota
  groups to `ProviderUsage` windows.
- `internal/agent/usage/client_gemini.go`: reads the Gemini CLI's stored OAuth
  token (read-only), resolves the Cloud project, and maps
  `retrieveUserQuota` buckets to model-scoped `ProviderUsage` windows.
- `internal/providerusage`: owns the observation store, the estimator, the
  backfill scanners, the single-flight index job, the service, and the HTTP
  handlers.
- `internal/backendapp`: constructs the service after `usageProviderAdapter`,
  wires the recorder, and injects a nil-safe quota-signal recorder into the
  lifecycle manager.
- `apps/web`: the `/usage` route, sidebar item, hooks, API client, and page
  components.

## Data model and persistence

`internal/providerusage` creates three tables with `CREATE TABLE IF NOT EXISTS`
and the same dialect-switched id pattern as
`internal/task/repository/sqlite/usage_events_schema.go`:

- `provider_usage_observations(id, account_key, provider, window_label, kind,
  utilization_pct, weighted_tokens, model, reset_at, observed_at, source)`.
  `kind` is `measured`, `tokens`, or `limit_hit`. A unique key on
  `(account_key, window_label, kind, source, observed_at, model)` makes
  re-indexing idempotent; token rows are upsert-added per hour. An index on
  `(account_key, observed_at)` serves the series read.
- `provider_usage_index_files(path_hash PRIMARY KEY, source, size, mtime,
  byte_offset, updated_at)`. The Kandev ledger cursor uses the fixed key
  `kandev:task_usage_events`, with `byte_offset` holding the last ledger id.
- `provider_usage_sources(source PRIMARY KEY, enabled, changed_at)`. No rows
  means every local source is off.

The repository exposes batch insert, list by account since a time, prune,
cursor get/put, and `ListSources`/`SetSource`/`DeleteSource`. `DeleteSource`
removes its observations and cursors in one transaction.

## Placement and routing

`apps/web/src/spa-routes.tsx` gains a `usage` case in `resolveTopLevelRoute`
and the shell title registry. `AppSidebarFixedNav`
(`components/app-sidebar/app-sidebar-primary-nav.tsx`) gains a Usage item with
`IconGauge` at its end, which places it between Inbox and New Task in both
layouts. The item shows a small dot when the last fetched overview had any
`nearing` or `exhausted` account; the sidebar itself does not poll. The mobile
navigation sheet renders the same component; if it does not, the destination is
added to the sheet's link list the way `OFFICE_SHEET_DESTINATIONS` does.

## HTTP surface

Routes register in the same authenticated group as
`POST /api/v1/agent-profiles/utilization`.

- `GET /api/v1/provider-usage?range=24h|7d|30d` (default `7d`). Lists profiles
  (`ListAgents` + `ListAgentProfiles`, cap 200 with `truncated`), fetches live
  values through the shared adapter with bounded concurrency and candidate-local
  errors, groups by provider and account, dedupes by account key, and returns
  status, measured windows, estimates, and a history series per window
  downsampled to at most 200 points, each tagged `measured`, `estimated`, or
  `limit_hit`.
- `GET /api/v1/provider-usage/index` returns
  `{state, files_total, files_done, started_at, finished_at, error_code,
  sources}`.
- `POST /api/v1/provider-usage/index` starts the single-flight job.
- `PUT /api/v1/provider-usage/sources` sets the local toggles (rejecting
  `kandev`) and applies the enable/disable effects.

## Sources and consent

`kandev` is always enabled and is not toggleable. `claude_local`,
`codex_local`, and `antigravity_local` default to off. A disabled local
source's scanner is never constructed, so its paths are never listed or read.
Enabling a source starts an incremental run; disabling deletes its observations
and cursors.

## Backfill

`providerusage/backfill_codex.go`, `backfill_claude.go`, and
`backfill_antigravity.go` each combine a streaming line parser over an
`io.Reader` with a `$HOME`-rooted path lister that skips a missing directory.
Codex transcript lines carry `payload.rate_limits` and produce `measured`
points using the same label function as the live client. Claude assistant
lines produce hourly weighted-token buckets (`input + output + cache_creation +
0.1*cache_read`). Antigravity CLI `RESOURCE_EXHAUSTED` lines produce
`limit_hit` points.

`providerusage/backfill_kandev.go` pages `task_usage_events` by `id > cursor`
in batches, maps each row's `agent_profile_id` through the adapter and falls
back to `agent_type`, and produces hourly `kandev` token buckets. Dedupe rule:
when `claude_local` is enabled the Anthropic estimator uses the transcript
series; otherwise it uses `kandev`.

`providerusage/index_job.go` is a single-flight runner with progress counters,
per-file cursors, and end-of-run pruning. A rerun with unchanged files is a
no-op. Parse errors are counted, never fatal.

## Estimation

`providerusage/estimate.go` is pure. Status and estimates follow
[`REQ-COSTS-PROVIDER-USAGE-002`](provider-usage-monitoring.md). Estimates use
the current window's measured points (same reset, else since the last drop).
Claude calibration derives `pct_per_weighted_token` from the latest live
snapshot and the weighted tokens since `reset_at - window`, applies it to
historical buckets, and always labels the result `estimated` with low
confidence. No calibration yields a tokens-only chart.

## Antigravity client

`client_antigravity.go` discovers the language server by scanning
`/proc/*/cmdline` on Linux (or `lsof` on macOS) for an executable containing
`language_server` with `--csrf_token` and `antigravity`, maps its listening
sockets, and calls
`POST http://127.0.0.1:<port>/exa.language_server_pb.LanguageServerService/GetUserStatus`
with `Connect-Protocol-Version: 1` and `X-Codeium-Csrf-Token`. It tries http
first, then https with verification skipped only for `127.0.0.1`. The CSRF
token is never logged. Process listing, port listing, and the HTTP client are
injectable so tests need no real process. No server yields `ErrSourceNotRunning`
and an `unavailable` account.

Quota groups map to one window each: `Claude …` to "Claude models",
`Gemini … Pro` to "Gemini Pro", `Gemini … Flash` to "Gemini Flash",
`GPT-OSS …` to "GPT-OSS", anything else to its own label.

## Model-scoped windows

`UtilizationWindow` gains an optional `Model` field. A window with an empty
`Model` applies to every profile; a window with a non-empty `Model` applies only
to a profile whose model matches (`window.Model == model` or
`strings.HasPrefix(model, window.Model)`). `RemainingPct(usage, model)` ignores
non-matching windows; an empty model argument considers all windows (the
pre-model behavior). When filtering leaves no windows, remaining is reported as
unknown, never as zero or full. The three callers (`agent/selection`,
`agent/settings` batch utilization, `agent/runtime/dynamic_resolver`) pass the
profile or candidate model when one is in scope, else `""`.

## Gemini Code Assist client

`client_gemini.go` reads `~/.gemini/oauth_creds.json` (`access_token`,
`refresh_token`, `expiry_date`) written by the operator's own `gemini` login.
It is read-only: no refresh and no write, because refreshing would require
borrowing the Gemini CLI's OAuth client credentials, which the design excludes.
An expired token (within 60 s of `expiry_date`) returns `ErrCredentialsExpired`
and the account is unknown or unavailable, never 0%. The project is resolved
once per client from `v1internal:loadCodeAssist`
(`cloudaicompanionProject`, a string or an object with `id`), overridden by
`GOOGLE_CLOUD_PROJECT`, and memoized in memory only. Quota is fetched from
`v1internal:retrieveUserQuota`; each bucket with a `remainingFraction` becomes
one model-scoped window and malformed buckets are skipped. No buckets means
windows `[]`, which reads as unknown. Errors carry a status code only; the
token, credential path, project id, upstream body, and email are never logged
or returned. The base URL is injectable so tests use `httptest`.

## OpenCode Go client

`client_opencode_go.go` reads a cookie through a backend-owned callback and
never receives a secret-store dependency. The adapter applies it only to
`opencode-acp` profiles whose model starts `opencode-go/`, sharing
`CacheKey("opencode-go", "console")`. The client resolves the organization
once (or uses `OPENCODE_ORG_ID`) and reads console status meters. Five-hour,
week, and month map to 5-hour, 7-day, and 30-day windows; month uses the access
end date as its reset. A missing or 401/403 cookie is unknown.

## Identity and attribution

The provider group and agent type are derived from the agent record's `name`
(the agent type, for example `codex-acp`) that the profile references through
`agents.id`, never from the profile's `agent_id` value itself. The usage adapter
resolves it with `settingsStore.GetAgent(ctx, profile.AgentID).Name` before any
registry lookup, billing decision, proxy resolution, or `ensureRegistered`
switch; the provider overview carries that name into `providerForAgent`. The
Gemini provider maps to `google` with the display name `Google`.

## Live and limit-hit recording

`providerusage.Service.RecordLive` is the recorder target. `RecordLive` inserts
a `measured` observation per window with a detached short context and logs
failures at debug. `RecordLimitHit` records a `limit_hit` observation. The
lifecycle manager's `classifyAndMaybeRemediate` calls an injected, nil-safe
`RecordLimitHit` when the classified code is `quota_limited` or
`rate_limited`, resolving the profile and carrying the reset hint.
The live adapter reads the newest future-reset limit hit after a fetch. Unless
a successful live value is newer than that hit, it appends a synthetic 100%
`limit` window and suppresses the fetch error. Repository errors fail open.

## Frontend

`lib/types/provider-usage.ts`, `lib/api/domains/provider-usage-api.ts`,
`hooks/domains/usage/use-provider-usage.ts`, and
`hooks/domains/usage/use-usage-index.ts` provide the types, client, and state.
`app/usage/usage-page.tsx` renders the indexing banner and progress, the
"History sources" card (Kandev always included, three opt-in switches), and one
card per provider. Provider cards use the existing `UtilizationBars` and an
inline SVG sparkline with measured solid, estimated dashed, and limit-hit
markers; no new chart dependency. All copy is translated in six locales, uses
no em dash, and status values are translation keys never compared after
translation.

## Failure and recovery

- A provider fetch error is account-local and never fails the page.
- A stale measured value older than the stale threshold marks the account
  `stale`.
- A missing Antigravity server marks the account `unavailable`.
- Index parse errors are counted and skipped.
- Recorder and limit-hit failures never affect the caller.

## Observability

The service logs index start/finish and recorder failures through the shared
logger. It never logs tokens, CSRF tokens, emails, or credential paths; only
hashed account keys are persisted or emitted.

## Related ADRs

- [Local provider usage sources and history backfill](../../decisions/2026-09-24-local-provider-usage-sources.md)
- [Gemini Code Assist quota from the Gemini CLI token](../../decisions/2026-09-26-gemini-code-assist-quota.md)
- [Limit-hit quota gate](../../decisions/2026-09-26-limit-hit-quota-gate.md)
- [OpenCode Go console quota](../../decisions/2026-09-26-opencode-go-console-quota.md)
- [Tagged, quota-aware initial workflow profile selection](../../decisions/2026-09-22-tagged-quota-agent-selection.md)

## Known limitations

See the requirements document. Backfilled Claude and Codex history is
attributed to the current host credential's account; Windows Antigravity
discovery and OpenCode in-band quota events are out of scope.

## Fork refinements (2026-09-25)

- `ProviderView.display_name` is resolved by `providerDisplayName`: the
  canonical map for the first-party providers, else the listed profile's
  `agent_display_name`/agent name, else the raw provider id.
- `ProviderAccount.label` is resolved by `accountLabel(plan, profiles,
  observations)`; `account_key` remains the React key and debug field.
- `ProviderAccount.quota_unavailable_reason` is populated when an account has
  no windows. `LiveProvider.QuotaUnavailableReason` mirrors the `GetUsage`
  skip branches; a pure classifier (`quotaUnavailableReasonFor`) keeps every
  branch testable.
- `WindowView.source` drives a provenance badge; `lastFetchedAt` reads the
  newest point in `History`. The frontend maps the closed set with
  `windowSourceBadge` and `quotaUnavailableReasonKey`.
- The page is chrome-consistent with Stats/GitHub via `PageShell`
  (`topbarTestId="usage-topbar"`); the E2E specs anchor on that testid.
