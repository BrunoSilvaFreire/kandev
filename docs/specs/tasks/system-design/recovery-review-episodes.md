---
status: draft
system: tasks
requirements:
  - REQ-TASKS-RECOVERY-REVIEW-001
---

# Recovery and review episodes system design

## Authority and existing boundaries

`internal/task/repository` owns durable task/session facts; orchestrator startup and periodic recovery supply liveness evidence but do not own a parallel inbox state. Keep the existing recoverable `WAITING_FOR_INPUT` session behavior and `tasks.metadata.interrupted_at` warning compatibility. The typed session episode is authoritative for reason and identity; the task marker is a compatibility projection, not a review signal. Clarification bundles remain authoritative under the Needs-you contract. See [ADR-2026-10-09-recovery-review-episodes](../../../decisions/2026-10-09-recovery-review-episodes.md).

## Persisted contract

Add nullable `task_sessions.interruption_episode_id`, `interruption_reason`, `interruption_observed_at`, and `interruption_turn_id`. The closed reason vocabulary starts with `backend_restart`, `execution_lost`, and `unknown_legacy`; provider quota/unavailable and execution failure retain their existing typed error owner and may be projected separately, not inferred from free text. One unresolved interruption episode belongs to one session. Allocate a new opaque episode ID only on the first successful conditional settlement of a lost active execution; a repeated sweep preserves ID and timestamp. A later distinct interruption after successful resolution gets a new ID. Store the source turn ID when known; never attach an unrelated turn.

Add nullable `tasks.review_episode_id`, `review_entered_at`, and `review_entry_origin` (`manual`, `workflow`, `agent_completion`, `unknown_legacy`, `observed_baseline`). The task repository's state-write transaction is the sole writer: on a real non-Review to Review transition, allocate ID and time with caller-supplied origin; while remaining in Review preserve both; on exit clear the active episode fields. All state write variants, including CAS, bulk/workflow, executor, and startup paths must use the same helper or be pinned by a writer-coverage test. Reject a direct Review write without an explicit origin or deliberately record `unknown_legacy`; never label an execution interruption as `agent_completion`. A review record is an active episode, not a history ledger.

## Recovery and marker lifecycle

Remove the startup path that coerces an interrupted `IN_PROGRESS` task to `REVIEW`; settle only the observed session/turn to recoverable waiting. Persist its interruption fields transactionally with the conditional session settlement (task archive, state, activity, and turn identity remain guards). A surviving execution and idle session create no episode. Publish task/session changes only after commit. If runtime work happens before the DB write, retry reconciliation idempotently; no transaction spans provider I/O.

Startup cleanup of a session already in `FAILED` also must not coerce its `IN_PROGRESS` task to `REVIEW`. Preserve the failed session and its existing typed execution/provider error for the inbox failure action; do not relabel that failure as an interruption or synthesize a Review episode. Cleanup of its runtime and executor record follows the existing resume-safety rules. An independent, genuine Review entry retains its own episode and is not cleared by startup cleanup. Cover the non-archived failed-session path and repeated startup in orchestrator tests.

Resolve an episode only after correlated provider-ready or accepted-running recovery for that session and expected episode ID. A stale callback cannot clear a newer episode. Recompute the task warning from all unresolved session episodes and legacy marker state; remove `interrupted_at` only when no unresolved episode remains. Preserve the existing warning DTO and UI icon contract. Never clear it merely on STARTING, focus, or an unsuccessful attempt.

## Migration and inbox projection

Use task-repository migrations for SQLite and PostgreSQL with same-database replay tests. For legacy `interrupted_at`, create an `unknown_legacy` session episode only if exactly one eligible session and its unresolved turn are provable; otherwise retain the task marker as an unattributed legacy interruption, with one task-level inbox incident and no invented session association. Legacy tasks already in Review get an `observed_baseline` episode at migration time; this timestamp means only that Review was observed then. They qualify as stale after the full age interval measured from that observation, never immediately due to old `updated_at`. Do not overwrite existing genuine review-entry data on replay.

Project one highest-priority actionable row per task/session incident: answerable clarification, interruption/resume, typed execution/provider failure, genuine Review, aged Review, external dependency, and advisory hints. The incident key is the episode ID (or stable task ID plus legacy marker for unattributed legacy data), not `updated_at` or a render-time category. A task with simultaneous session episodes may expose distinct session actions, but the task badge/count deduplicates by task and resolves precedence deterministically. A Review episode with an unresolved interruption is not presented as a stale review. A visible-tab timer or bounded refresh recomputes the stale threshold from server-provided review time; task updates do not reset it. Dismissal/acknowledgement keys include the episode ID, so a later episode is not hidden by an earlier dismissal.

The task DTO and `task.updated` carry `interruption` (`episode_id`, `reason`,
`session_id` of the oldest unresolved session episode, explicit `null` when
none), `review_episode_id`, and `review_entered_at`. One row is shown per task.
Precedence is clarification, interruption, aged Review, possible question,
Review, dependency wait. A fresh Review and a dependency wait are listed but do
not raise the navigation nudge, because they are standing states rather than
new incidents. The Failed tab keeps its own rows and adds `reason_kind`
(`quota` or `provider_unavailable`), derived only from the session's typed
agent-error code.

Statements that write `tasks.state` outside the task repository's transactional
helper embed the shared `internal/task/reviewepisode` SET clause, or reconcile
with it after the write in the same transaction. A source scan test fails when
a new writer does neither.

## Integration and safety

Task DTOs and HTTP/WS expose additive reason, episode, and review-entry fields; old clients retain `interrupted`. Keep raw provider errors and inaccessible task details out of user copy. The existing inbox triage utility consumes the typed projection and shares categories with desktop and phone. Desktop and phone provide the same reason and resume/retry/review action; phone uses the established task drawer and accessible tap targets, not hover. Verify repeated restart, stale callback, multiple sessions, archive race, review entry/exit/re-entry, legacy baseline, timer ageing, clarification precedence, and replay on both databases.
