---
status: active
system: costs
created: 2026-09-24
owners:
  - kandev
---

# Provider Usage Monitoring Requirements

## Overview

Kandev can fetch live subscription quota windows for Claude and Codex, but the
values are surfaced only inside Office cards and a settings batch endpoint, and
the history behind them is discarded. An operator cannot see, on one page,
which provider accounts exist, how full each window is, how fast it is filling,
or when it last filled.

This capability gives the costs system one provider-usage view: a top-level
`/usage` page reached from the sidebar, which reads the ledger Kandev already
writes for its own sessions and, when the operator opts in, imports history
from local Claude, Codex, and Antigravity CLI files. It extends
[`REQ-COSTS-SUBSCRIPTION-USAGE-001`](subscription-usage.md) rather than
replacing it: the live clients, cache, and consumer contracts stay unchanged.

## Terminology

- **Account**: one provider credential, keyed by
  `agentusage.CacheKey(provider, credentialLocator)`. Several profiles can
  share one account.
- **Measured observation**: a utilization percentage the provider itself
  reported, with the window it belongs to.
- **Token observation**: a weighted token count attributed to an hour, model,
  and account.
- **Limit hit**: a classified quota or rate-limit failure with an optional
  reset hint.
- **Estimated series**: percentages derived by calibration from token
  observations, never reported by a provider.
- **Local source**: a host file tree that is read only after the operator
  enables it (`claude_local`, `codex_local`, `antigravity_local`).

## Requirements

### REQ-COSTS-PROVIDER-USAGE-001: Provider usage page

**Intent:** The operator needs one place to see the current state of every
provider account Kandev knows about.

#### Acceptance criteria

- **AC-COSTS-PROVIDER-USAGE-001.1:** The web application exposes a top-level
  `Usage` destination, reachable from the sidebar between Inbox and New Task in
  every layout (Kanban and Office, collapsed and expanded) and from the mobile
  navigation sheet, without requiring Office mode.
- **AC-COSTS-PROVIDER-USAGE-001.2:** Opening the page fetches live utilization
  for the current host's agent profiles and groups it by provider and account,
  deduplicating profiles that share one credential.
- **AC-COSTS-PROVIDER-USAGE-001.3:** A provider account shows its plan, the
  profiles that use it, a utilization bar per measured window, and a reset
  countdown.
- **AC-COSTS-PROVIDER-USAGE-001.4:** A provider account shows a status of
  `exhausted`, `nearing`, `healthy`, `stale`, `unknown`, or `unavailable`. The
  provider status is the worst status among its accounts.
- **AC-COSTS-PROVIDER-USAGE-001.5:** When Antigravity is not running, the page
  shows that it is not running and does not fail the page.

### REQ-COSTS-PROVIDER-USAGE-002: Usage history and estimates

**Intent:** A single number cannot show whether a window is filling fast. The
page needs the past, and a projection, without inventing provider data.

#### Acceptance criteria

- **AC-COSTS-PROVIDER-USAGE-002.1:** Each measured window has a history series
  drawn from `measured` observations and, where calibration is available, an
  `estimated` series. Estimated points are always labelled as estimated.
- **AC-COSTS-PROVIDER-USAGE-002.2:** An estimate carries a burn rate, a
  projected exhaustion time, a trend, an optional anomaly flag, and a
  confidence of `none`, `low`, or `medium`.
- **AC-COSTS-PROVIDER-USAGE-002.3:** When a live fetch fails or the source is
  not running and the last measured observation is older than the stale
  threshold, the account shows `stale`.
- **AC-COSTS-PROVIDER-USAGE-002.4:** Observations older than the retention
  window are pruned.

### REQ-COSTS-PROVIDER-USAGE-003: History sources and consent

**Intent:** Reading a user's other AI tools' local files is a privacy decision
that must belong to the user, while Kandev's own recorded usage is already
Kandev data.

#### Acceptance criteria

- **AC-COSTS-PROVIDER-USAGE-003.1:** Kandev's own session usage ledger is
  indexed by default and cannot be disabled.
- **AC-COSTS-PROVIDER-USAGE-003.2:** Local Claude, Codex, and Antigravity CLI
  history sources are disabled by default; no path under their directories is
  listed or read while disabled.
- **AC-COSTS-PROVIDER-USAGE-003.3:** Enabling a source starts an index run for
  it. Disabling a source deletes the observations and cursors that came from
  it.
- **AC-COSTS-PROVIDER-USAGE-003.4:** The index job is resumable per file, skip
  unchanged files, and never fail the run because one line cannot be parsed.
- **AC-COSTS-PROVIDER-USAGE-003.5:** While indexing, the page shows progress
  and states that indexing is in progress.

### REQ-COSTS-PROVIDER-USAGE-004: Antigravity and unified history

**Intent:** Antigravity exposes its quota through an already-running local
language server, and every classified quota failure is history worth keeping.

#### Acceptance criteria

- **AC-COSTS-PROVIDER-USAGE-004.1:** Antigravity utilization is read from the
  running local language server over loopback using its CSRF token. When no
  server is found the account is `unavailable`.
- **AC-COSTS-PROVIDER-USAGE-004.2:** Antigravity groups are reported as one
  window per quota group (Claude models, Gemini Pro, Gemini Flash, GPT-OSS, and
  any other label group).
- **AC-COSTS-PROVIDER-USAGE-004.3:** Every successful live fetch from any
  consumer records a measured observation for the account.
- **AC-COSTS-PROVIDER-USAGE-004.4:** A classified `quota_limited` or
  `rate_limited` agent failure records a limit hit for the owning profile when
  one can be resolved.
- **AC-COSTS-PROVIDER-USAGE-004.5:** The page never displays a token, CSRF
  token, email, or credential path.

## Exclusions

- Antigravity cloud-OAuth session access and borrowed OAuth clients; only the
  local language server is read. Gemini Code Assist quota (below) reads the
  token the Gemini CLI already stored and borrows no client.
- Estimated absolute remaining tokens or requests; providers expose no
  denominator.
- Notifications and background polling; the page fetches only while open.
- OpenCode Go `opencode-go/*` profiles use the account-wide console status
  endpoint with an operator-supplied global `opencode-console-cookie`; other
  OpenCode providers remain out of scope.
- Windows Antigravity discovery; Windows reports the source as unavailable.

## Known limitations

- Backfilled Claude and Codex history is attributed to the current host
  credential's account because the transcripts carry no account identity.
- With only Kandev-sourced tokens, work done outside Kandev is invisible, so a
  calibrated percentage can overstate cost per token; it stays labelled
  estimated with low confidence.

## Fork refinements (2026-09-25)

### REQ-COSTS-PROVIDER-USAGE-005: human-readable identities and provenance

The usage surfaces present provider/account identities and quota provenance
without raw identifiers, and explain why an account has no live quota.

- **AC-COSTS-PROVIDER-USAGE-005.1:** The `/usage` page is wrapped in
  `PageShell`; the range selector and refresh live in the topbar and the body
  scrolls, so a large provider/account list is reachable on desktop and phone.
- **AC-COSTS-PROVIDER-USAGE-005.2:** A provider entry carries a
  `display_name` (a canonical name for `anthropic`/`openai`/`antigravity`,
  otherwise the agent registry display name, otherwise the raw id). An account
  entry carries a `label` derived at read time: plan plus first linked profile
  name, then plan, then first profile name, then a local-history label for the
  `claude_local`/`codex_local` sources. `account_key` stays in the payload as
  debug-only.
- **AC-COSTS-PROVIDER-USAGE-005.3:** The task Usage panel never renders a
  profile UUID or a bare session id as its row label; it falls back to a
  translated "Deleted profile"/session label and keeps the short id only in a
  tooltip.
- **AC-COSTS-PROVIDER-USAGE-005.4:** Each window shows an explicit provenance
  badge ("Reported by provider" for measured, "Estimated from local history"
  for the estimate series) plus a last-fetched time derived from the newest
  observation in the series.
- **AC-COSTS-PROVIDER-USAGE-005.5:** An account with no live client carries a
  closed-set `quota_unavailable_reason` (`api_key_billing`,
  `no_provider_endpoint`, `profile_missing`) taken from the adapter's skip
  branch, replacing the bare "No quota source" text with the actual reason.

## Fork refinements (2026-09-26)

### REQ-COSTS-PROVIDER-USAGE-006: Gemini Code Assist quota

**Intent:** Gemini subscription accounts are as opaque to the scheduler as Codex
was. Reading the token the Gemini CLI already stored recovers their measured
quota without a fresh login and without borrowing anyone's OAuth client.

#### Acceptance criteria

- **AC-COSTS-PROVIDER-USAGE-006.1:** Gemini quota is read from
  `~/.gemini/oauth_creds.json` (`access_token`, `refresh_token`,
  `expiry_date`). The token is read only: Kandev never refreshes it, never
  writes the file, and never borrows the Gemini CLI's OAuth client credentials.
  An expired token yields an unknown or unavailable account, never 0%.
- **AC-COSTS-PROVIDER-USAGE-006.2:** The Google Cloud project comes from
  `v1internal:loadCodeAssist` (`cloudaicompanionProject`, a string or an object
  with `id`) and is overridden by `GOOGLE_CLOUD_PROJECT` when set. Quota comes
  from `v1internal:retrieveUserQuota`; each bucket that carries a
  `remainingFraction` becomes one window whose utilization is
  `clamp((1 - remainingFraction) * 100)`. Malformed buckets are skipped.
- **AC-COSTS-PROVIDER-USAGE-006.3:** A bucket that names a model only affects
  profiles whose model it applies to. A profile whose model does not match the
  bucket sees no window for it; when filtering leaves no windows the account is
  unknown rather than healthy.
- **AC-COSTS-PROVIDER-USAGE-006.4:** The provider and the agent type are
  resolved from the agent record (`agents.name`) the profile references, never
  from the profile's `agent_id` value itself, which is the `agents.id` UUID.
- **AC-COSTS-PROVIDER-USAGE-006.5:** Errors and logs carry only a status code;
  the token, credential path, project id, upstream body, and account email are
  never logged or returned.

### REQ-COSTS-PROVIDER-USAGE-007: Limit hits and OpenCode Go

- **AC-COSTS-PROVIDER-USAGE-007.1:** An unexpired classified limit hit makes
  its account read as 0% remaining until reset unless a newer successful live
  fetch supersedes it. A zero reset is ignored.
- **AC-COSTS-PROVIDER-USAGE-007.2:** An `opencode-go/*` profile uses shared
  OpenCode Go console meters. A 100% meter is exhausted even if prepaid
  balance could keep requests serving.
- **AC-COSTS-PROVIDER-USAGE-007.3:** Missing or expired console credentials
  are unknown, never exhausted. The cookie is never logged or returned.

## Fork refinements (2026-09-28)

### REQ-COSTS-PROVIDER-USAGE-008: window chips and usage breakdown

**Intent:** The per-window metadata line is a run of bare text, and the page
cannot answer which sessions or models are consuming the recorded usage. Both
are reframed as first-class, explorable surfaces.

#### Acceptance criteria

- **AC-COSTS-PROVIDER-USAGE-008.1:** Each piece of window metadata (provenance,
  reset, last fetched, burn rate, trend, projected exhaustion, anomaly) renders
  as a distinct chip, and every chip exposes a tooltip with the detail behind
  it. The chips are keyboard-focusable, and a series with too few points
  renders a single "not enough data" chip instead of a fabricated estimate.
- **AC-COSTS-PROVIDER-USAGE-008.2:** The `/usage` page exposes a Breakdown tab
  alongside Quotas. The active tab is mirrored in the URL as `?tab=breakdown`
  so it survives reloads and can be linked, and the topbar range selector
  applies to both tabs.
- **AC-COSTS-PROVIDER-USAGE-008.3:** The Breakdown tab groups the Kandev
  session ledger by provider, model, agent type, task, session, or UTC day, and
  can filter by provider, model, agent type, task, session, and free text over
  task title, session name, model, agent type, and provider. Group-by, filter
  and sort values are closed whitelists mapped to SQL fragments; user input is
  never interpolated.
- **AC-COSTS-PROVIDER-USAGE-008.4:** Breakdown rows report token totals
  (in/out/cached), cost, event count, the share of the filtered total, and
  first/last activity; columns sort by tokens, cost, events, or last activity
  in either direction, and the result set paginates.
- **AC-COSTS-PROVIDER-USAGE-008.5:** A breakdown row drills down: selecting it
  adds that row as a filter and advances to the next dimension (provider to
  model to session; agent, task, or day to session). Active filters render as
  removable chips with one action that clears them all.
- **AC-COSTS-PROVIDER-USAGE-008.6:** On a phone-width viewport the Breakdown
  table becomes stacked row cards, the filters move behind a Filters control in
  a sheet, sort becomes a select, and the page never overflows horizontally.
