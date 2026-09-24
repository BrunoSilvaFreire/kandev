---
id: task-05
title: Prove user flows and update docs
status: done
wave: 4
depends_on:
  - task-01
  - task-02
  - task-03
  - task-04
plan: docs/plans/tagged-quota-agent-selection/plan.md
requirements:
  - REQ-AGENTS-TAGGED-QUOTA-SELECTION-001
acceptance_criteria:
  - AC-AGENTS-TAGGED-QUOTA-SELECTION-001.12
  - AC-AGENTS-TAGGED-QUOTA-SELECTION-001.13
  - AC-AGENTS-TAGGED-QUOTA-SELECTION-001.14
system_design:
  - docs/specs/agents/system-design/tagged-quota-agent-selection.md
---

# Task 05: Prove user flows and update docs

## Outcome

Desktop and phone end-to-end specs prove tag persistence and quota-ranked
selection, and public plus fork docs describe the opt-in behavior.

## Scope

In scope: focused settings/workflow E2E specs, `docs/public/agents-and-profiles.md`,
`docs/public/workflow-tips.md`, `docs/public/feature-status.md` when needed, and
`docs/fork-evaluation.md`.

Excluded: behavior changes and unrelated suites.

## Requirements and design

- `REQ-AGENTS-TAGGED-QUOTA-SELECTION-001`
- [System design](../../specs/agents/system-design/tagged-quota-agent-selection.md)

## Acceptance

- Tags persist through reload on desktop and phone.
- A step saves two allowed tags and a fallback.
- Mock utilization launches the higher-remaining tagged profile; a backend test
  proves the equal-quota tie.
- No match uses the fallback; no fallback blocks move/launch with visible
  recovery.
- The phone flow uses the picker sheet, 44px targets, no horizontal overflow,
  and the same outcome.
- Tests use mock usage only, never developer credentials or live providers.

## Verification

```bash
(cd apps/web && pnpm e2e:run tests/settings/agent-profile-layout.spec.ts tests/workflow/tagged-quota-agent-selection.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-agent-profile-layout.spec.ts tests/workflow/mobile-tagged-quota-agent-selection.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check -- docs/public docs/fork-evaluation.md
```

Stop: confirm Playwright discovers the intended desktop/mobile tests before
treating green output as evidence.

## Likely files and risks

`apps/web/e2e/tests/settings/`, `apps/web/e2e/tests/workflow/`, and the public
and fork docs above.

Risk: E2E reset fixtures and shard limits; do not overlap full suites.

## Results

Docs: added profile-tag and allowed-tags guidance to
`docs/public/agents-and-profiles.md` and `docs/public/workflow-tips.md`, and a
divergence-log row to `docs/fork-evaluation.md`. `validate-public-docs` passes.

E2E (passing): extended `tests/settings/agent-profile-layout.spec.ts` with a
canonical tag persistence test, and added
`tests/workflow/tagged-quota-agent-selection.spec.ts` plus the mobile
`tests/settings/mobile-agent-profile-layout.spec.ts` and
`tests/workflow/mobile-tagged-quota-agent-selection.spec.ts`. Verified on host
Chromium and `mobile-chrome`.

Gap: the quota-driven launch E2E ("mock utilization launches the higher-remaining
tagged profile" and the equal-quota tie in the browser) is not implemented,
because there is no mock subscription-usage provider in the E2E runtime. It
needs a small env-gated usage client hook before it can be tested end to end;
the deterministic ranking/tie/fallback logic is covered by
`internal/agent/selection` unit tests.

### Review fix round

The gap is closed:
- Added an env-gated mock usage provider (`KANDEV_E2E_MOCK`) in
  `usage_adapter.go`: a `mock-quota-<pct>` profile tag yields deterministic
  utilization, so E2E can pick winners without live providers.
- `tagged-quota-agent-selection.spec.ts` now proves the utilization endpoint
  reports known remaining percentages and that a new entry freezes the
  higher-remaining tagged profile in `workflow_session_route`.
- Added focused client tests (`agent-profile-utilization-api.test.ts`) and hook
  tests (`use-agent-profile-utilization.test.ts`) covering success, error,
  cancellation, and no-fetch-when-closed.
- Phone tag removal now meets 44px on coarse pointers, asserted by the mobile
  spec; the previously deleted mobile regressions are restored.

### Review fix round 2

- Candidate utilization now batches requests at the endpoint's 50-ID limit and
  merges results; a failed batch marks only its own candidates `unavailable`
  and surfaces a `candidateError` status. Tests: `chunkProfileIds`,
  multi-batch fetch, and failed-batch-unavailable in
  `use-agent-profile-utilization.test.ts`.
- Client tag validation now measures UTF-8 bytes (`TextEncoder`) to match the
  backend cap; a multibyte test covers a tag that passes UTF-16 length but
  exceeds 64 bytes.

### Review fix round 3

- Message-dispatch rollback now freezes a fresh route for a restored tagged
  step in the same transition (`Service.attachRollbackEntryRoute`), so a
  restore cannot commit a tagged entry that fails closed on launch. Tests:
  `message_rollback_entry_route_test.go`.
- The candidate preview is independent of editability: read-only workflows
  still load quota and show candidate states, and the phone flow renders the
  candidates through `MobilePickerSheet` with focus return and 44px targets.
  Tests: `workflow-step-allowed-tags.test.tsx`, mobile E2E.
- E2E now proves the no-match fallback and the no-safe-fallback block:
  `tagged-quota-agent-selection.spec.ts`; the exhausted error surfaces as a
  visible 400 with its message via `isValidationError`.
- `TagTokenInput` canonicalizes before the count cap; the obsolete
  non-error-returning orchestrator resolver was removed and its tests now use
  the entry-aware resolver.
- The step editor and `docs/public/agents-and-profiles.md` explain that
  enabling or changing allowed tags applies to the next entry.

### Review fix round 4

- The exhausted-candidate desktop E2E now proves visible recovery: after the
  blocked entry it opens the step editor and asserts the candidate preview
  renders `0% remaining`, alongside the API rejection.
- The phone E2E now proves the same selection outcome as desktop (a new entry
  freezes the higher-remaining tagged profile) and asserts the candidate
  trigger and sheet close control render at least 44px.
- Direct repository writes canonicalize tags at both persistence boundaries
  (`agent/settings/store` `enrichmentValues`, `workflow/repository`
  `marshalAllowedTags`) and reject invalid tags, covered by
  `sqlite_tags_test.go` and `allowed_tags_test.go`.
