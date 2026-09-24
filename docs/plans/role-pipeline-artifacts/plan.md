---
spec: docs/specs/tasks/requirements/kanban-task-documents.md
created: 2026-09-22
status: complete
---

# Implementation Plan: Role Pipeline approval gate and durable Spike artifact

Two outcomes for the fork's Role Pipeline workflow:

1. A configurable human approval gate between Architect and Implement, so no
   plan reaches implementation without a person moving the card.
2. A durable Spike report stored as a task document with key `spike`, readable
   by the Architect step and by a human in the Kanban task view, so the Spike
   stops owning the task implementation plan.

The gate is workflow configuration and changes no product contract. The artifact
requires widening two existing consumer surfaces of `task_documents`.

Follow-up (Kandev task `5ad0573e`): `REQ-TASKS-KANBAN-DOCUMENTS-004` (keyed
same-type documents, `new_revision` plan revisions) and the conditional Plan
Approval bypass on escalation in `docs/examples/role-pipeline.workflow.yml`.

## Requirements and design

- [Requirements](../../specs/tasks/requirements/kanban-task-documents.md) —
  `REQ-TASKS-KANBAN-DOCUMENTS-001..003`
- [System design](../../specs/tasks/system-design/kanban-task-documents.md)

## Work orders

- [Task 01](task-01-plan-approval-gate-and-prompts.md): insert the configurable
  Plan Approval gate step in the Role Pipeline example and rewrite the Spike and
  Architect prompts around the `spike` document. Configuration and docs only.
- [Task 02](task-02-task-mode-document-tools.md): register the task-document MCP
  tools in task mode and default `task_id` to the calling task.
- [Task 03](task-03-kanban-document-surface.md): mount task-scoped, authorized
  document routes and render documents in the Kanban plan panel.

## Dependency order

Task 02 and Task 03 are independent of each other and both independent of
Task 01 at the code level. Task 01 is only useful once Task 02 has shipped,
because the rewritten prompts call the document tools; sequence it last when
delivering in one pass.

## Risks

- **Authorization regression.** Mounting the Office document handler on
  `/api/v1` without a per-task guard would expose other users' documents. The
  guard is the point of Task 03 and its denial test is not optional.
- **Existing workflows are not migrated.** The live Role Pipeline lives in the
  workspace database. Editing the example file does not change an imported
  workflow, and editing a step does not reach a session already in that step.
  The gate must be added before a task reaches Architect, or the card moved by
  hand.
- **Gate deadlock by design.** A gate step with no `on_enter` auto-start and no
  `on_turn_complete` transition parks the task until a human moves it. That is
  the intent; it is also a way to lose a task if nobody watches the column.

## ASCII UI preview

`UI-01: Kanban task, Plan panel, documents section` — entry point
`/t/:id` (advanced mode), Plan tab focused, task has one spike document.

```text
+-- Plan ---------------------------------------------------+
|                                                           |
|  # Implementation plan                                    |
|  1. ...                                                   |
|  2. ...                                                   |
|                                                           |
|-----------------------------------------------------------|
|  Documents                                    [ + New ]   |
|                                                           |
|  > [SPIKE]  ACP step advancement spike                    |
|             agent - updated 2h ago                        |
|  > [NOTES]  Manual repro log                              |
|             you - updated 10m ago                         |
+-----------------------------------------------------------+
```

The plan content region scrolls; the documents section follows it in the same
scroll container. Expanding a row reveals rendered markdown in place. Empty
state renders the existing "no documents" copy, not an error.

Phone composition is unchanged from the existing simple view: the same
`TaskDocuments` component, full width, one card per row. No separate phone
design is introduced because no new layout is introduced.

Structural requirements: the documents section sits below the plan, uses the
existing `TaskDocuments` component and its type badges, and adds `SPIKE` to the
badge table. Spacing and column widths above are illustrative.

## Verification

Per work order. Overall: backend `make -C apps/backend test lint`, web
`pnpm --filter @kandev/web lint` and the targeted vitest files, then
`python3 scripts/list-docs.py validate` and
`python3 scripts/lint-spec-files.py --all`.
