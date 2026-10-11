# ADR-2026-10-09-transition-failure-diagnostics: Classify transition failures once

**Status:** proposed
**Date:** 2026-10-09
**Area:** protocol

## Context

The MCP move handler enriches one completion-gate error, while other move and completion paths emit bare codes, generic messages, or logs only. HTTP, WebSocket, and UI paths make separate classification decisions. Asynchronous completion signals and deferred moves can be accepted before a later failure, so an immediate response cannot be the only diagnostic record.

## Decision

The task domain owns one typed transition-failure classifier and safe public envelope. Adapters serialize that envelope without reclassifying the error. Asynchronous failures persist by attempt identity and emit a task-scoped diagnostic event for reconnect and later agent context. The UI parses the envelope once and presents the same reasons across move surfaces. Existing codes and successful response shapes remain stable.

## Consequences

Each new transition guard must return a typed cause or attach a rule identifier so the shared classifier can generate a reason and remedy. Unknown errors receive a safe internal-failure record with an incident ID. The backend must preserve authorization boundaries while enriching failures; the frontend must localize known codes and retain fallback rendering for older servers.

## Alternatives Considered

- Per-handler enrichment repeats classification and drifts across MCP, HTTP, WebSocket, and UI.
- Passing raw error strings can leak internals and cannot provide stable remediation.
- Returning success for accepted asynchronous work without a later outcome leaves failed transitions invisible after reconnect.
