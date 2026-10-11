---
status: draft
system: tasks
created: 2026-10-09
owners:
  - kandev
---

# Session step visits

## Overview

A session has one editable task-step visit association for navigation and grouping. Its original creation step and workflow route remain immutable historical evidence.

## Requirements

### REQ-TASKS-SESSION-VISITS-001: Associate sessions with evidenced visits

**Intent:** Keep sessions findable under the right occurrence of a workflow step, including re-entry and retired steps, without inventing pre-activation history.

- **AC-TASKS-SESSION-VISITS-001.1:** A visit identifies one task's entry into one workflow step. Re-entry has a distinct identity. Current and historical visits survive restart and step retirement.
- **AC-TASKS-SESSION-VISITS-001.2:** New sessions created by manual start, handoff, or recovery receive the current visit when one exists. A pre-workflow session may remain unassigned.
- **AC-TASKS-SESSION-VISITS-001.3:** A user may reassign a session to an existing eligible visit of the same task; the original creation-step and route records do not change. A visit from another task or a retired step as a new assignment is rejected.
- **AC-TASKS-SESSION-VISITS-001.4:** Legacy sessions are automatically assigned only when durable evidence uniquely identifies an evidenced visit. Ambiguous or pre-activation sessions remain unassigned, not guessed into a reconstructed visit.
- **AC-TASKS-SESSION-VISITS-001.5:** Step history, activity, and session tabs use the same association. Repeated visits are distinguishable on desktop and phone; historical retired-step associations remain readable.

## Exclusions

This feature does not backfill `task_step_transitions`, infer historical step boundaries before ledger activation, alter `session_step_history`, or create a cost-attribution record. It does not change the independent workflow-step progress indicator.
