# ADR-2026-09-22-tagged-quota-agent-selection: Tagged, quota-aware initial workflow profile selection

**Status:** accepted
**Date:** 2026-09-22
**Area:** backend

## Context

A workflow step pins exactly one concrete `agent_profile_id`. An operator who
wants "whichever reviewer subscription still has headroom" must either edit the
step by hand or run every review through one profile until its quota is spent.
No existing mechanism matches a step to a set of interchangeable profiles.

Dynamic agent routing already owns post-launch provider fallback, circuit
breaking, generations, and recovery, and is reached from a different seam. Its
[accepted ADR](2026-08-13-dynamic-agent-profile-routing.md) explicitly deferred
telemetry-backed selection. Task and workflow session lifecycle already freeze
one profile per workflow entry through `WorkflowSessionRoute.AgentProfileID`.

The profile model already carries `role`, but that is a closed Office
organizational enum, not a free-form user label, and the profile row already
persists JSON-array TEXT columns (`skill_ids`, `desired_skills`).

## Decision

A concrete agent profile carries free-form `tags`, and a workflow step may
declare `allowed_tags`. At each workflow-step entry the backend selects one
eligible concrete profile by remaining subscription quota and freezes it for
that entry.

- Tags are canonicalized at every write boundary (trim, lowercase, reject
  empty, deduplicate, sort, cap count and per-tag byte length) and stored as
  JSON-array TEXT defaulting to `[]`.
- `allowed_tags` uses OR semantics; a profile qualifies when it has at least one
  allowed tag. A multi-tag profile is one candidate.
- When `allowed_tags` is empty, resolution is unchanged: fixed task override,
  step profile, workflow default, then existing fallbacks. Empty tags are the
  operational kill switch.
- The selector chooses only the initial concrete profile. Dynamic routing
  remains the sole owner of post-launch retry and fallback and is not modified.
- Scoring is `remaining_pct = clamp(100 - max(window.utilization_pct), 0, 100)`,
  ranking known-positive descending, then unknown/unavailable, then known-zero,
  with canonical profile ID as the tie-break. A provider error is
  candidate-local unavailable, never a selection failure.
- Selection is evaluated once per entry. The freeze record is the existing
  bounded `task.metadata[workflow_session_route]` value, keyed by the committed
  transition identity `entry:%020d` derived from `task_step_transitions.id`.
  The transition transaction fills the entry identity and writes the route with
  the ledger row, so a manual move, engine/CAS transition, genesis creation, or
  WIP/feeder promotion that commits a tagged destination always carries a
  frozen choice. A later entry has a different identity and may replace the
  bounded record; restart, replay, and reuse never re-rank it.
- The read-only `workflow_move_preview` may run the same selector for advisory
  disclosure but never creates an entry or reserves a route. Guaranteeing
  preview-to-launch identity would require a separate reservation lifecycle and
  is out of scope.
- One narrow `Select(ctx, candidates) (Decision, error)` strategy boundary ships
  only `QuotaStrategy`. Tag filtering belongs to the selector service.

This decision is implemented by the
[tagged quota selection package](../plans/tagged-quota-agent-selection/plan.md).

## Consequences

- New additive `agent_profiles.tags` and `workflow_steps.allowed_tags` columns
  default to `[]`; rollback leaves inert columns and clearing `allowed_tags`
  restores legacy behavior without a runtime flag.
- Unknown telemetry is a normal, first-class state rather than an error.
  API-key and unsupported providers stay selectable.
- A cold provider call can add up to the existing 10-second timeout when the
  candidate surface is opened; settings boot never calls providers.
- The freeze reuses the existing bounded route record instead of adding a
  second metadata key, table, or in-process memo. A pending route template is
  carried through the transition context and persisted atomically with the
  transition ledger row; if the transition loses a CAS race or is diverted, the
  pending route is not written.
- Profile resolution has many callers, so every actual entry mutation must
  persist a pending route and every launch path must consume the recorded
  profile rather than re-score.

## Alternatives Considered

- Reusing Office `role` would overload a closed organizational enum and break
  Office semantics.
- A second post-launch fallback engine duplicates dynamic routing and would race
  its circuits and generations.
- A persisted strategy enum, registry, or factory has no second strategy to
  justify the indirection.
- Treating unknown quota as full quota would prefer a provider with no known
  headroom over a known-good one; treating it as zero would strand API-key and
  unsupported providers.
- Re-ranking between preflight and launch could select a different profile than
  the one the user previewed and the session recorded.
- A tag join table and index are unnecessary for bounded per-profile tag lists.
