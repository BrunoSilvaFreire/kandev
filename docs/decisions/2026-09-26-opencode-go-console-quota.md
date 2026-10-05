# ADR-2026-09-26-opencode-go-console-quota: Read console meters

**Status:** accepted
**Date:** 2026-09-26
**Area:** backend

## Context

OpenCode Go API credentials are rejected by the console status endpoint. The
browser console session exposes account-wide five-hour, weekly, and monthly
meters instead.

## Decision

Read the undocumented console endpoint using an operator-copied global cookie
secret named `opencode-console-cookie`. Do not persist, log, or return the
cookie. A missing or expired cookie makes quota unknown; a classified limit hit
remains the fallback.

## Consequences

The cookie expires and operators must renew it. Meters are account-wide, so no
model filter applies.

## Alternatives Considered

- Use the Go API key. Rejected because the console endpoint returns 403.
- Show prepaid balance as quota. Rejected because it is spending credit, not
  subscription headroom.
