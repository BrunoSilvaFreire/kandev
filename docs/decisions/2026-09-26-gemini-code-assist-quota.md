# ADR-2026-09-26-gemini-code-assist-quota: Gemini Code Assist quota from the Gemini CLI token

**Status:** accepted
**Date:** 2026-09-26
**Area:** backend

## Context

Kandev tracks live subscription quota for Claude, Codex, and a locally running
Antigravity. Gemini subscription accounts stay opaque: the Gemini agent reports
`api_key` billing and no quota, so quota-aware scheduling treats every Gemini
candidate as unknown and falls back to configured order.

The Gemini CLI already authenticates with Google and stores the resulting OAuth
token at `~/.gemini/oauth_creds.json`. The Cloud Code Assist endpoints
`v1internal:loadCodeAssist` and `v1internal:retrieveUserQuota` return the
account's tier and per-model quota for that token.

A prior boundary excluded `cloudcode-pa.googleapis.com` entirely, because the
Antigravity cloud path required a separate Google login and carried
account-suspension risk, and because refreshing the token would mean borrowing
the Gemini CLI's OAuth client id and secret.

## Decision

Add one read-only Gemini Code Assist quota client.

- **Read the operator's existing token; borrow nothing.** Kandev reads
  `~/.gemini/oauth_creds.json` exactly as the CLI wrote it. It never refreshes
  the token, never writes the file, and never uses the CLI's OAuth client
  credentials. An expired token is terminal for the fetch: the account is
  reported unknown or unavailable, never 0%. This keeps the suspension/borrowing
  boundary intact while still recovering the quota the operator already granted
  their own CLI.
- **Project from `loadCodeAssist`, quota from `retrieveUserQuota`.**
  `cloudaicompanionProject` (a string or an object with `id`) is the project,
  overridden by `GOOGLE_CLOUD_PROJECT` when set and memoized in memory. Each
  `retrieveUserQuota` bucket with a `remainingFraction` becomes one model-scoped
  window.
- **Model-scoped windows.** `UtilizationWindow` gains an optional `Model`, and
  the remaining-utilization rule ignores a window whose `Model` does not match
  the profile's model. A bucket therefore only affects the profiles it applies
  to; a filter that leaves no windows is unknown, not healthy.
- **Resolve the agent type from the agent record.** Every usage-path lookup uses
  `settingsStore.GetAgent(ctx, profile.AgentID).Name`, not `profile.AgentID`,
  which is the `agents.id` UUID. This is a prerequisite: without it no
  Codex/Claude/Gemini client is reachable and the ranking falls back to
  configured order.
- **Privacy.** Errors and logs carry a status code only; the token, credential
  path, project id, upstream body, and account email are never logged or
  returned.

## Consequences

- Gemini subscription profiles become measurable, so quota-aware scheduling can
  skip an exhausted Gemini candidate instead of treating it as unknown.
- The decision is explicitly not to add a refresh path. When the CLI's token
  expires, Gemini quota goes unknown until the operator logs in again. This is
  visible (an unavailable/unknown account), not silent, and never fabricates a
  zero.
- `billing_type` for Gemini becomes `subscription` when the credential file
  carries a token, so the agent reads as a subscription account everywhere the
  billing type is surfaced.

## Alternatives

- **Refresh the token in Kandev.** Rejected: it requires the Gemini CLI's OAuth
  client id and secret, the exact borrowing the boundary excludes.
- **A separate Google login for Kandev.** Rejected: the Antigravity cloud path
  was already excluded for this reason; the operator using the CLI already
  provides a token.
- **Treat an expired token as 0% remaining.** Rejected: it would wrongly
  disable every Gemini candidate.
- **Per-model windows with no model filter.** Rejected: a bucket for a model
  the profile does not use would cap an unrelated profile's remaining quota.
