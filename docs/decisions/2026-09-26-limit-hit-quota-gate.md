# ADR-2026-09-26-limit-hit-quota-gate: Preserve known quota exhaustion

**Status:** accepted
**Date:** 2026-09-26
**Area:** backend

## Context

A provider fetch can fail immediately after an agent reports a quota limit.
Treating that account as unknown lets scheduling retry the exhausted account.

## Decision

Use the newest limit-hit observation with a future reset as a synthetic 100%
utilization window. A newer successful live fetch wins; missing or zero resets
do not gate routing.

## Consequences

Scheduling has a safe fallback without a provider poll. The gate naturally
expires at the provider reset.

## Alternatives Considered

- Treat every fetch error as exhausted. Rejected because outages must not
  disable healthy accounts.
- Keep the gate only in dynamic routing. Rejected because every consumer needs
  the same account truth.
