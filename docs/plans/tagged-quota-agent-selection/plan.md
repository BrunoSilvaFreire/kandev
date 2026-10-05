---
spec: docs/specs/agents/requirements/tagged-quota-agent-selection.md
created: 2026-09-22
status: superseded
---

# Implementation Plan: Tagged quota-aware workflow agent selection

> **Superseded (2026-09-25):** schedule-time ranking is now owned by
> [Dynamic Profile Scheduling](../../specs/agents/requirements/dynamic-profile-scheduling.md).
> The workflow-step `allowed_tags` scheduler described here is retained only as
> the compatibility bridge and is removed after the migration and parity gates
> pass. See [the ownership ADR](../../decisions/2026-09-25-dynamic-profile-scheduling.md).

Add free-form tags to concrete agent profiles, let a workflow step declare
`allowed_tags`, and choose one eligible concrete profile at each workflow-step
entry using remaining subscription quota. Freeze that choice for the entry,
preserve fixed-profile behavior when tags are absent, and show tags plus quota
state in the existing settings selectors on desktop and phone.

## Requirements and design

- [Requirements](../../specs/agents/requirements/tagged-quota-agent-selection.md)
  — `REQ-AGENTS-TAGGED-QUOTA-SELECTION-001`
- [System design](../../specs/agents/system-design/tagged-quota-agent-selection.md)
- [ADR: tagged quota-aware initial workflow profile selection](../../decisions/2026-09-22-tagged-quota-agent-selection.md)

## Work orders

- [x] [Task 01](task-01-profile-tags.md) (`done`): persist and edit canonical concrete
  profile tags through backend storage, API, and the desktop/phone editor.
- [x] [Task 02](task-02-step-allowed-tags.md) (`done`): carry `allowed_tags` through every
  workflow create/update/duplicate/export/import/sync/MCP path and the step
  selector UI.
- [x] [Task 03](task-03-batch-usage.md) (`done`): normalize the remaining-quota score and add
  the shared Office-independent batch-utilization endpoint and client hook.
- [x] [Task 04](task-04-quota-selection-freeze.md) (`done`: AC-001.16 fallback fix): add the selection strategy,
  wire error-aware resolution, and freeze one choice per entry.
- [x] [Task 05](task-05-e2e-and-docs.md) (`done`): prove desktop/phone flows end to end and
  update public and fork docs.

Task 04 architecture checkpoint: keep the pending-route context handoff into
`recordStepTransition`. That repository chokepoint derives the route identity
from the committed ledger row and persists `WorkflowSessionRoute` in the same
transaction. Do not add a second persistence key, an in-memory memo, or quota
selection inside the repository.

## Dependency order

Task 01 and Task 02 are independent persistence changes. Task 03 depends on
Task 01 (candidate tags) only at the wiring level. Task 04 depends on Task 02
(step input) and Task 03 (scoring). Task 05 depends on Tasks 01–04. Within one
delivery pass, implement in numeric order.

## Risks

- **Preview/launch divergence.** Profile resolution has many callers. Missing
  one splits the previewed profile from the launched profile. Every
  `resolveStepAgentProfileForTask*` caller must use the error-aware selector.
- **Unknown telemetry is normal.** Treating unknown as zero strands API-key and
  unsupported providers; treating it as full wrongly outranks known quota.
- **Cold provider latency.** A single provider call can take the existing
  10-second client timeout; concurrency avoids multiplying it by candidate
  count.
- **Dirty-tree overlap.** Unrelated in-flight changes touch agent registration
  and backend composition. Reconcile narrowly and preserve them.
- **Additive columns.** SQLite cannot drop columns cleanly, so migrations must
  be additive and code rollback safe.

## ASCII UI preview

`UI-01: Agent profile editor, Tags row` — entry point Settings → Agent
Profiles → edit a concrete profile. Desktop keeps 28px primitives.

```text
+-- Agent profile ------------------------------------------+
|  Name         [ Claude Reviewer                    ]       |
|  Agent        [ Claude                              v ]    |
|  Model        [ claude-sonnet-4                     v ]    |
|  Tags                                                      |
|  [ review x ] [ security x ] [ type tag...          ]      |
+------------------------------------------------------------+
```

Phone uses the same token list; the input row is at least 44px and occupies
full width.

`UI-02: Workflow step selector, Allowed tags + candidate preview` — entry point
Settings → Workflows → step → Agent profile. Desktop Popover.

```text
+-- Workflow step: Review ----------------------------------+
|  Allowed tags                                              |
|  [ review x ] [ security x ] [ type tag...          ]      |
|                                                            |
|  Fallback profile                                          |
|  [ Claude • Reviewer                              v ]      |
|                                                            |
|  Candidate preview                                         |
|  Claude • Reviewer                  72% remaining          |
|  Codex • Reviewer                   Quota unavailable      |
+------------------------------------------------------------+
```

Phone replaces the dropdown with `MobilePickerSheet` and single-scroll body:

```text
+-- Workflow step: Review ----------------------------------+
|  Allowed tags                                              |
|  [ review x ] [ security x ]                               |
|  [ Add tag................................... ]            |
|                                                            |
|  Fallback profile                                          |
|  [ Claude • Reviewer                          ]            |
|                                                            |
|  +------------------------------------------+             |
|  | Agent candidates                         |             |
|  | Claude • Reviewer        72% remaining   |             |
|  | Codex • Reviewer         Unavailable     |             |
|  |                          [ Done ]        |             |
|  +------------------------------------------+             |
+------------------------------------------------------------+
```

Structural requirements: tags are shown as removable tokens; the candidate
preview is read-only; known, unknown, unavailable, empty-match, loading, and
known-zero states must not rely on color alone; phone targets are at least 44px
and use the existing `MobilePickerSheet` single scroll owner and focus return.
Spacing above is illustrative, not a pixel specification. Mapped to
`AC-AGENTS-TAGGED-QUOTA-SELECTION-001.1`, `.2`, `.3`, `.4`, `.7`, `.13`, `.14`.

## Verification

Per work order. Final integration gate:

```bash
make -C apps/backend build
make -C apps/backend lint
(cd apps/web && pnpm run typecheck)
(cd apps && pnpm --filter @kandev/web lint)
(cd apps/web && pnpm run i18n:check)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Do not overlap full E2E suites or override repository shard/worker limits.

## Rollback

- Migrations are additive `TEXT NOT NULL DEFAULT '[]'`; code rollback leaves
  inert columns. Do not drop them.
- Clearing `allowed_tags` restores legacy fixed-profile behavior and is the
  operational kill switch.
- Preserve all unrelated dirty-tree changes. Never rewrite Office `role`,
  dynamic route state, or historical session profile IDs.
