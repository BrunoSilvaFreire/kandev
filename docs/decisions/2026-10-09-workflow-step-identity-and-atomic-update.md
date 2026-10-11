# ADR-2026-10-09-workflow-step-identity-and-atomic-update: Retain step identity during workflow updates

**Status:** proposed
**Date:** 2026-10-09
**Area:** backend

## Context

Workflow sync matches steps by name and writes workflow fields and individual steps separately. A rename can allocate a new step ID; a failed later write can leave a partial definition. Task transitions and routes retain step IDs, while step participants have a cascade foreign key to `workflow_steps`.

## Decision

The workflow repository owns a workflow-scoped, opaque portable key and nullable retirement marker on each step. Portable export v3 includes the key; v1/v2 remain readable with unique-name matching for unchanged steps and explicit mapping for renames. Removing a step from a file retires its row after a live-reference check. Active queries exclude retired rows; historical resolution uses the retained row and original ID. A single workflow-repository transaction checks a canonical preview digest and source ownership, rechecks references, and applies the entire definition. Competing PostgreSQL task/queue/session writes must share the workflow-row lock protocol before retirement is enabled there.

The workflow subsystem owns one idempotent migration helper for the new columns, key backfill, and index. Task repository initialization calls it after creating its workflow-step projection and before built-in inserts; workflow repository initialization calls the same helper after creating its table. This preserves task-only stores and the production order in which task storage initializes first, without maintaining two copies of the migration.

## Rationale

Retaining the row preserves existing step, participant, and decision identity without copying a second historical record. The portable key survives renames and independent copies across workspaces. One transaction prevents a partially applied workflow and stale-preview confirmation.

## Alternatives

- **Match by name or position:** renames are ambiguous and can misassociate current tasks.
- **Delete and create a separate tombstone/snapshot:** duplicates historical state and needs participant FK migration.
- **Keep service-level sequential writes with compensating rollback:** cannot reliably undo side effects or concurrent references.

## Consequences

All active step readers must filter retired rows, while history readers must retain access. Normal delete and Git sync paths need reconciliation with the same retention and transaction contract. The migration and transaction must support SQLite and PostgreSQL; no retirement is allowed on PostgreSQL until reference-creating writers obey the lock protocol.
