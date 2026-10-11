# ADR-2026-10-09-session-step-visits: Task-owned visit projection

**Status:** accepted
**Date:** 2026-10-09
**Area:** backend

## Context

The transition ledger is forward-looking and forbids reconstructing missing historical transitions. `workflow_step_entries` exists only for selected Office dispatch entries. Session navigation needs a persistent, editable association to each occurrence of a task step without changing either historical source.

## Decision

The task repository owns a visit projection sourced from committed destination transitions and a clearly marked current-state activation baseline where no recorded entry exists. It writes future visits atomically with task moves and ledger rows. Legacy session association is automatic only for a unique evidenced interval; otherwise it stays null. The ledger, session route, and creation-step stamp are never backfilled or rewritten. A retired step retains its visit and label for reading but accepts no new association.

## Consequences

Old ambiguous sessions remain unassigned and a baseline does not claim a historical entry timestamp. This adds a task-owned table and migration but gives re-entry a durable identity across restart and workflow edits. New writes must share the task transaction and locking boundary.

## Alternatives Considered

- Use `workflow_step_entries`: rejected because manual moves do not allocate them.
- Use `task_step_transitions` IDs alone: rejected because a current task may have no recorded entry at ledger activation.
- Reconstruct historical transitions from timestamps/routes: rejected because this invents task-level history and violates the ledger contract.
