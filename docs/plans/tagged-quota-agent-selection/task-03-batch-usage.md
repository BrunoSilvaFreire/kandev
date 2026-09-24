---
id: task-03
title: Normalize quota and add shared batch usage
status: done
wave: 2
depends_on:
  - task-01
plan: docs/plans/tagged-quota-agent-selection/plan.md
requirements:
  - REQ-AGENTS-TAGGED-QUOTA-SELECTION-001
acceptance_criteria:
  - AC-AGENTS-TAGGED-QUOTA-SELECTION-001.8
  - AC-AGENTS-TAGGED-QUOTA-SELECTION-001.9
  - AC-AGENTS-TAGGED-QUOTA-SELECTION-001.13
system_design:
  - docs/specs/agents/system-design/tagged-quota-agent-selection.md
---

# Task 03: Normalize quota and add shared batch usage

## Outcome

One shared usage adapter serves Office, the workflow selector, and a new
authenticated bounded batch endpoint that reports `known | unknown |
unavailable` with optional raw utilization and remaining percentage.

## Scope

In scope: the remaining-quota score helper in `internal/agent/usage`, the
`usageProviderAdapter` in `internal/backendapp`, backend composition, the batch
endpoint and its client hook, and candidate-row display states.

Excluded: the selection algorithm, step fields, and provider-registration
refactor.

## Requirements and design

- `REQ-AGENTS-TAGGED-QUOTA-SELECTION-001`
- [System design](../../specs/agents/system-design/tagged-quota-agent-selection.md)

## Acceptance

- The score uses the most-utilized window, clamps to `0..100`, and treats
  nil/empty windows as unknown.
- The batch endpoint bounds and validates input and distinguishes the three
  states.
- Duplicate credential bindings share the cache; fetches are concurrent and
  cancelable with the request.
- Office and non-Office paths share one adapter.
- The candidate UI loads on open; settings boot never calls providers.

## Verification

```bash
(cd apps/backend && go test ./internal/agent/usage/... ./internal/backendapp/...)
(cd apps/web && pnpm exec vitest run components/settings/workflow-step-agent-profile-selector.test.tsx lib/api/domains)
```

Stop: telemetry failure remains candidate-local. Never fail settings boot or
workflow entry solely because telemetry is unavailable.

## Likely files and risks

`internal/agent/usage/`, `internal/backendapp/usage_adapter.go`, backend
composition, agent-settings handler wiring, `lib/types/agent-profile.ts`, a
usage hook, selector rows, tests, and locales.

Risk: composition is touched by unrelated dirty-tree changes; reconcile
narrowly and keep one adapter instance.

## Results

Score helper done: `usage.RemainingPct` (most-utilized window, clamped,
nil/empty unknown) with `remaining_test.go`.

Backend endpoint done: `POST /api/v1/agent-profiles/utilization` in
`internal/agent/settings/handlers` (request `{"profile_ids":[...]}`;
trim/dedupe; reject empty and >50 unique; concurrent bounded fetch under the
request context; per-profile `known | unknown | unavailable` with optional
`remaining_pct`; deterministic profile-ID order). The shared
`usageProviderAdapter` is now constructed outside the Office conditional in
`internal/backendapp/main.go` and injected into Office (when present) and the
settings handlers. Tests: `profile_utilization_handlers_test.go`,
`profile_utilization_routes_test.go`.

Frontend done: `agent-profile-utilization-api.ts`,
`useAgentProfileUtilization` (loads only when the candidate surface opens),
and the candidate preview in `WorkflowStepAllowedTags` rendering known,
unknown, unavailable, loading, and empty states, with `workflows` copy in all
six locales.

Remaining: none for settings boot (it never calls providers).
