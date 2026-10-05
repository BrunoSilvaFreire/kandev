# ADR-2026-09-24-local-provider-usage-sources: Local provider usage sources and history backfill

**Status:** accepted
**Date:** 2026-09-24
**Area:** backend, frontend

## Context

Kandev fetches live Claude and Codex quota windows and surfaces them only in
Office. There is no account-wide view, no history, and no Antigravity support.
Providers expose utilization as a percentage with no denominator, so history is
the only way to show whether a window is filling fast. Prior transcripts of
Kandev and of the local Claude, Codex, and Antigravity CLIs already contain
measurements and token activity that could seed that history.

Reading another tool's local files is a privacy decision. Kandev's own session
ledger is already Kandev data and does not carry the same consent requirement.

## Decision

One new backend package, `internal/providerusage`, owns a durable provider-usage
model, a backfill index job, an estimator, and the `/usage` HTTP surface. The
live clients in `internal/agent/usage` remain the only live fetch path and gain
an optional fetch recorder.

- **One observation model.** `provider_usage_observations` stores `measured`,
  `tokens`, and `limit_hit` rows under a hashed account key equal to the live
  cache key. Every consumer that fetches live feeds it through one recorder.
- **Local sources are opt-in per source.** `claude_local`, `codex_local`, and
  `antigravity_local` default to off and are never listed or read while off.
  The `kandev` source is always on and not toggleable. Disabling a source
  deletes the observations and cursors derived from it. Live quota clients are
  unchanged: they continue to read credential files by default, and the
  Antigravity probe reads only a loopback process and its response.
- **Local language server over cloud OAuth for Antigravity.** The running
  Antigravity language server is queried over loopback with its CSRF header.
  The `cloudcode-pa.googleapis.com` cloud path, which requires a separate
  Google login and carries account-suspension risk, is excluded.
- **Estimated values are always labelled.** Percentages derived from token
  calibration are marked `estimated` with low confidence and are never merged
  into the measured series.

## Consequences

- Backfilled Claude and Codex history is attributed to the current host
  credential's account, because transcripts carry no account identity. This is
  a recorded limitation.
- Kandev-only tokens cannot see usage outside Kandev, so a calibrated
  percentage can overstate cost per token; it stays labelled estimated.
- The workflow quota strategy now treats `agy-acp` profiles as known, because
  the adapter registers an Antigravity client for them.

## Alternatives

- **Cloud OAuth for Antigravity.** Rejected: separate Google login and a
  documented suspension risk.
- **Always-on local history reads.** Rejected: reading other tools' files must
  be a user decision.
- **A parallel poller.** Rejected: the live cache stays the single fetch path;
  a second poller would double provider calls and diverge from the cache.
- **A separate history table per source.** Rejected: one observation model with
  a `source` column keeps the estimator and prune logic single.
