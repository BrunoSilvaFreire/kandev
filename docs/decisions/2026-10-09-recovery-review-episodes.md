# ADR-2026-10-09-recovery-review-episodes: Separate interruption and Review episodes

**Status:** accepted
**Date:** 2026-10-09
**Area:** backend

## Context

`tasks.metadata.interrupted_at` indicates restart loss but does not identify the affected session or reason. Startup currently moves an interrupted task to Review; inbox triage treats that marker as stale Review and otherwise ages Review using `tasks.updated_at`. These facts conflate execution loss, review intent, and unrelated edits.

## Decision

The task repository owns active typed interruption episodes on sessions and active Review episodes on tasks. Conditional recovery writes the session episode once; a genuine task-state transition into Review writes its episode in the same transaction. The task interruption marker remains a compatibility warning projection. Inbox classification reads these facts and clarification authority; it does not persist another state machine. Legacy Review age starts at an explicitly labelled observation baseline when historical entry time is unknown.

## Consequences

The schema adds nullable fields and requires all task-state writers to share episode logic. Multiple session interruptions remain distinguishable, while a task-level warning remains compatible. Historical age is conservative and may delay a stale label, but never fabricates a review request or date.

## Alternatives Considered

- Extend task metadata alone: rejected because several sessions can be interrupted independently and task metadata cannot safely resolve a single session episode.
- Add a separate inbox incident table: rejected because it would duplicate task/session lifecycle authority and need cross-table reconciliation.
- Infer from `updated_at`, error text, or Review state: rejected because these change for unrelated reasons and cannot prove review intent or entry time.
