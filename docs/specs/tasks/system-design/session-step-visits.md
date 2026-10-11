---
status: draft
system: tasks
requirements:
  - REQ-TASKS-SESSION-VISITS-001
---

# Session step visits system design

## Ownership and authority

The task repository owns visit identity and the editable session association. `task_step_transitions` remains the immutable source of task-level step boundaries under its existing forward-looking contract; `workflow_step_entries` is an optional Office dispatch record allocated only on some transitions and cannot be the universal visit table. A transition's initiating `session_id` is not the owner of that visit. `session_step_history`, route rows, and `workflow_step_id_at_creation` remain unchanged. See [ADR-2026-10-09-session-step-visits](../../../decisions/2026-10-09-session-step-visits.md).

## Data and write boundary

Add task-owned `task_step_visits` with a task-scoped unique identity, workflow ID, step ID, entry ordinal, `entered_at`, nullable unique `source_transition_id`, and provenance (`transition` or `activation_baseline`). A foreign key to the task cascades on task deletion; no foreign key to a workflow step may erase a retired/deleted historical label. A transition-backed visit corresponds to exactly one committed ledger row whose destination is a step. On every future step change, create the visit in the same task-repository transaction as the task update and ledger append. A no-op move creates none. Use ledger ID ordering, not wall-clock ordering, to assign ordinals. A detach leaves no current visit.

Add a nullable `task_sessions.associated_step_visit_id` (or an equivalent one-row task-owned association table) with a composite task/visit constraint. Creation paths insert the session and association in one transaction after reading the task's current visit under the task write lock; handoff and recovery must use that common path. Reassignment takes expected association/version, validates same task and visit eligibility, and changes only this association. Concurrent step move versus session creation serializes at the task row; after commit the session must point to the visit current at its creation transaction, not an earlier read. A historical retired-step association remains readable but retirement forbids assigning another session to it. Step-primary selection is separate.

## Migration and legacy evidence

The migration never inserts or changes a `task_step_transitions` row. For each existing destination transition, idempotently materialize at most one visit with that transition ID as source; this is a projection of recorded facts, not a historical transition backfill. A task currently in a step with no evidenced entry receives one `activation_baseline` visit at migration time, explicitly labelled as an activation snapshot, with unknown historical entry time. It must not imply the task entered the step then or before then. Do not create visits for earlier intervals for which no transition row exists. A later committed transition creates a normal visit.

Legacy session association is conservative: a durable route or creation-step stamp may narrow candidates, but automatic assignment requires exactly one compatible evidenced visit and an unambiguous interval from recorded ledger boundaries. An initiating-session ID alone is insufficient because the transition belongs to the task. A session predating the first recorded boundary, crossing an unknown interval, having conflicting evidence, or matching multiple re-entries stays null. The activation-baseline visit is available for new sessions after migration, not an automatic destination for old sessions. Keep null legacy sessions visible in an Unassigned group with their original evidence; do not silently hide them. Migration is repeatable, does not rewrite original evidence, and tests SQLite and PostgreSQL replay and rollback.

## Read and interaction contract

The task-scoped API returns visit identity, current/historical status, provenance, step label (retained step or historical fallback), entry ordinal, and associated session IDs. The reassignment endpoint accepts a visit ID and expected association/version; step-only choice resolves to the latest eligible current visit only when unambiguous, otherwise requests an explicit visit choice. The transition summary, step hover, and session-tab menu consume this projection. The phone session menu uses the existing touch-accessible sheet pattern and exposes the same visit/provenance labels and reassignment choices as desktop. Deleted or inaccessible steps retain a safe historical label without offering reassignment.

## Failure and verification

Reject cross-task IDs, stale versions, retired-step new assignments, and missing current visits with typed errors. Migration stops rather than guessing on duplicate transition IDs or inconsistent task/visit rows. Tests cover forward transitions, re-entry, no-op, detach, bootstrap, legacy ambiguity, restart, retirement, creation paths, concurrent creation/move, and desktop/phone selection.
