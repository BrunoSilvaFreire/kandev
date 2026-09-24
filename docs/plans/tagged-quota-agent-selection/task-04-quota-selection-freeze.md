---
id: task-04
title: Select once and freeze the workflow-entry profile
status: done
wave: 3
depends_on:
  - task-02
  - task-03
plan: docs/plans/tagged-quota-agent-selection/plan.md
requirements:
  - REQ-AGENTS-TAGGED-QUOTA-SELECTION-001
acceptance_criteria:
  - AC-AGENTS-TAGGED-QUOTA-SELECTION-001.4
  - AC-AGENTS-TAGGED-QUOTA-SELECTION-001.6
  - AC-AGENTS-TAGGED-QUOTA-SELECTION-001.10
  - AC-AGENTS-TAGGED-QUOTA-SELECTION-001.11
  - AC-AGENTS-TAGGED-QUOTA-SELECTION-001.12
system_design:
  - docs/specs/agents/system-design/tagged-quota-agent-selection.md
---

# Task 04: Select once and freeze the workflow-entry profile

## Outcome

A tag-configured step selects one eligible concrete profile by remaining quota
at entry, persists it with the entry's route and session, and never re-ranks
that entry, while fixed and dynamic-routing behavior is unchanged.

## Scope

In scope: the new `internal/agent/selection` package, the error-aware
`resolveStepAgentProfileForTask*` wiring and all its callers, route/session
freezing, and the structured selection log.

Excluded: post-launch fallback, dynamic routing internals, and step/API fields.

## Requirements and design

- `REQ-AGENTS-TAGGED-QUOTA-SELECTION-001`
- [System design](../../specs/agents/system-design/tagged-quota-agent-selection.md)

## Acceptance

- OR and multi-tag matching, canonical matching, and invalid-candidate
  exclusions behave as specified.
- Ranking is highest remaining, then profile-ID tie-break, then
  positive/unknown/zero ordering; a fetch error stays candidate-local.
- No-match falls back; all-zero uses a safe fallback or returns the typed error;
  a missing fallback returns the typed error.
- A task fixed-step override wins.
- Actual manual/engine/genesis/promotion entry preflight produces one pending
  route template; `recordStepTransition` persists it atomically with the new
  ledger identity in `task.metadata[workflow_session_route]`.
- The entry-aware resolver reads the exact route before scoring; legacy
  fixed-profile resolution remains unchanged.
- Existing-session and no-session launch paths consume the same recorded
  profile; restart/replay/reuse never re-rank it.
- Read-only `workflow_move_preview` is advisory and performs no write; a new
  committed entry re-evaluates quota.
- Dynamic conductor behavior is unchanged.

## Verification

```bash
(cd apps/backend && go test ./internal/agent/selection/... ./internal/orchestrator/...)
(cd apps/backend && go test ./internal/agent/runtime/dynamic/...)
(cd apps/backend && go test ./internal/workflow/... ./internal/task/models/...)
```

Stop if one selection per entry cannot be proven with the existing route record.

## Likely files and risks

`internal/agent/selection/`, `internal/orchestrator/{service,event_handlers_workflow,workflow_session_target,workflow_move_preview,session_ensure,task_operations}.go`,
backend composition, and focused tests.

Risk: a missed caller splits preview from launch.

## Results

Selection core done: `internal/agent/selection` (`Matches`/`FilterCandidates`
OR semantics, `Strategy` boundary, `QuotaStrategy` positive > unknown > zero,
max-remaining descending, profile-ID tie-break, concurrent candidate-local
scoring, all-exhausted signalling) with full `selection_test.go`.

Revision 1 freeze wiring done:
- `internal/workflow/entryroute` carries the pending route on context and owns
  the `entry:%020d` identity / deterministic operation-ID format; the
  orchestrator's route/identity helpers now delegate to it.
- `internal/task/repository/sqlite.recordStepTransition` persists a
  destination-matching pending route into `task.metadata[workflow_session_route]`
  in the same transaction as the ledger row, and discards a diverted/raced
  pending route. Proven by
  `step_transitions_pending_route_test.go`.
- The task service resolves one concrete profile at actual entry
  (`MoveTaskWithOptions` and `createTaskWithCapacity` for genesis/WIP) through
  `WorkflowEntryProfileSelector` and attaches the pending route; the adapter in
  `internal/backendapp` lists global enabled concrete candidates, applies the
  strategy, and falls back to the step profile only when it is not exhausted.
- The entry-aware orchestrator resolver reads the exact persisted route by
  destination step and entry identity before any scoring, returns the typed
  `entryroute.ErrNoFrozenProfile` when a tagged entry has no matching route, and
  keeps legacy fixed-profile resolution unchanged. (Superseded for the
  no-match case by the Architect decision below.)

### Architect decision: no-route fallback (AC-001.16)

The durable requirement AC-001.16 overrides the earlier fail-closed plan text
and review guidance. Implementer scope:
- `resolveStepAgentProfileForTaskError` (`internal/orchestrator/event_handlers_workflow.go`):
  keep the order task fixed-step override, then `resolveTaggedEntryProfile`
  (pending or exact persisted route). On no match, return
  `strings.TrimSpace(step.AgentProfileID)` when it is non-empty, and
  `entryroute.ErrNoFrozenProfile` otherwise. Do not use the workflow default,
  call the selector, or persist a route. Emit one Warn log with the task ID,
  step ID, and profile ID.
- `previewStepAgentProfile` is unchanged, because the preview models a new
  entry through the selector.
- Tests: rename `TestResolveTaggedEntryProfileFailsClosedEvenWithStepProfile`
  to `...FallsBackToStepProfileWithoutRoute` and assert
  `step-fallback-profile` with no selector call. Keep
  `MissingRouteFailsClosed` for an empty step profile. Change
  `RequiresMatchingStartPolicy` to use an empty step profile, or assert the
  fallback. Keep `TaskOverrideWinsWithoutRoute` as it is.

Implemented: `resolveStepAgentProfileForTaskError` now trims and returns the
step's `agent_profile_id` when no pending/exact route matches, logs one Warn
with the task/step/profile IDs, and never calls the selector, consults the
workflow default, or persists a route. Tests:
`TestResolveTaggedEntryProfileFallsBackToStepProfileWithoutRoute` (trimmed
fallback, zero selector calls, no route written), with `MissingRouteFailsClosed`
and `RequiresMatchingStartPolicy` covering the empty-profile typed error.

### Review fix round

Addressed the reviewer's blockers 1-4 and 8:
- Engine/CAS: `applyEngineTransitionWithCommitMode` resolves and attaches the
  pending route before credential preflight and before either the normal or
  guarded CAS commit; the orchestrator now receives the shared selector.
- Orchestrator queue promotions (`pullOneFeederTask`, `promoteSameStepTask`)
  attach selection before every promotion write.
- `previewStepAgentProfile` runs the selector read-only for tagged steps and
  emits a `tag_quota_advisory` notice; it writes no route.
- Candidate eligibility reuses `ValidateAgentProfileForExecutor` through the
  adapter's validator, excluding executor-incompatible profiles.
- Restored the deleted `mobile-agent-profile-layout.spec.ts` regressions.

Tests: `workflow_entry_route_engine_test.go`,
`workflow_store_entry_route_test.go`, `workflow_move_preview_test.go`
(read-only tag selection), and the executor-compatibility case in
`backendapp/workflow_entry_profile_selector_test.go`.

### Review fix round 2

- Blocked tag-configured steps from being `session_target.kind=step` sources at
  every validation surface: orchestrator `validateWorkflowSessionTargetSource`,
  workflow controller `validateSessionTarget`, service import
  `validateWorkflowSessionTargets`, and the frontend earlier-step filter.
  Tests: `workflow_session_target_test.go` (tagged source),
  `session_target_source_validation_test.go`, and the selector component test
  that no tagged earlier step is offered.
- Move preview now returns the tag selector's result/error directly for tagged
  steps (no fallthrough to defaults) and emits a `tag_profile_unavailable`
  notice; tests cover the selector error and the nil-selector typed error.
- Engine/CAS: `attachEngineEntryRoute` takes the active session, prefers the
  session's executor, and fails closed on a task read error; the attacher
  signature threads the session through promotions. Tests cover session-executor
  preference and task-read failure.

Architect verification on 2026-09-23: keep this wiring. The pending route is
resolved before the write, while `recordStepTransition` alone owns the committed
ledger identity and atomically persists the bounded `WorkflowSessionRoute`.
The targeted selection, orchestrator, dynamic-routing, workflow, task-model,
task-service, SQLite-repository, and backend-composition Go suites passed.
