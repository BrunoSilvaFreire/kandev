---
status: draft
system: tasks
created: 2026-10-09
owners:
  - kandev
---

# Recovery and review episodes

## Overview

Restart interruption is an execution outcome, not a request for human review. The task system records the cause and episode identity of interrupted sessions and genuine Review entries so inbox projections can explain the right action and measure age without relying on unrelated task edits.

## Requirements

### REQ-TASKS-RECOVERY-REVIEW-001: Durable interruption and Review facts

**Intent:** Preserve the distinction between recoverable execution loss and a task genuinely awaiting review across restarts.

- **AC-TASKS-RECOVERY-REVIEW-001.1:** A lost active execution shall leave its session recoverable and carry a durable typed interruption reason, observed time, and stable episode identity. Repeated reconciliation of the same episode shall not create another incident or reset its age.
- **AC-TASKS-RECOVERY-REVIEW-001.2:** Restart recovery alone shall not move a task to Review or create a review episode. A genuine transition into Review shall record its own episode identity, entry time, and cause; edits while in Review shall not reset its age.
- **AC-TASKS-RECOVERY-REVIEW-001.3:** Successful correlated session recovery shall resolve only that session's interruption episode. A failed, blocked, or merely started attempt shall not resolve it. An unresolved sibling shall keep the task interruption warning.
- **AC-TASKS-RECOVERY-REVIEW-001.4:** Legacy interruption and Review records shall remain readable. Where their cause, session owner, or entry time cannot be proven, the system shall label them unknown or observed-at-migration rather than claim an invented historical fact.
- **AC-TASKS-RECOVERY-REVIEW-001.5:** The inbox shall present a truthful reason and next action for each actionable episode, prioritize answerable clarifications, and avoid duplicates on restart. A review becomes stale only after its authoritative entry or conservative observation time exceeds the configured age; time passing shall refresh the view without another task event.

## Exclusions

This contract does not resume all agents at startup, synthesize review requests from interruption, alter clarification authority, or use `tasks.updated_at` as Review age.
