---
id: task-01
title: Plan Approval gate step and role prompts
status: complete
wave: 2
depends_on:
  - task-02
plan: docs/plans/role-pipeline-artifacts/plan.md
requirements: []
acceptance_criteria: []
system_design: []
---

# Task 01: Plan Approval gate step and role prompts

## Outcome

The Role Pipeline example parks every task on a human-owned step between
Architect and Implement, and its Spike and Architect prompts use the `spike`
task document instead of the task implementation plan.

## Scope

In scope: `docs/examples/role-pipeline.workflow.yml`, the Spike prompt in
`docs/examples/spike-design.workflow.yml` if it shares the wording, and the fork
run log in `docs/fork-evaluation.md`.

Excluded: any Go or TypeScript change, any mutation of a workflow already
imported into a workspace database, and the `requires_approval` step field,
which stays dormant because nothing sets `review_status=pending`.

## Requirements and design

Configuration only. No `REQ-*` applies; the gate changes no product contract.
The prompt rewrite depends on the tools delivered by Task 02.

## Acceptance

- A `Plan Approval` step sits at position 3 between Architect and Implement with
  no agent profile, no `on_enter` action, and no `on_turn_complete` action, and
  with `allow_manual_move: true`. Later steps are renumbered.
- The Spike prompt writes its report with `write_task_document_kandev` using key
  `spike` and type `spike`, and explicitly does not write the task plan.
- The Architect prompt reads `spike` with `get_task_document_kandev`, states
  what to do when the document is absent (proceed from the task prompt and name
  the gap rather than invent evidence), and remains the only owner of the task
  implementation plan.

## Verification

```
python3 scripts/list-docs.py validate
git diff --stat docs/examples docs/fork-evaluation.md
```

Manual: re-import the example through Settings > Workspaces > Workflows >
Import into a scratch workspace and confirm the board shows six columns with
Plan Approval between Architect and Implement.

## Likely files and risks

`docs/examples/role-pipeline.workflow.yml`, `docs/examples/spike-design.workflow.yml`,
`docs/fork-evaluation.md`.

Risk: a task already sitting in Architect in an existing workspace will not gain
the gate. State the re-import and manual-move path in the run log.

## Results

Done. `role-pipeline.workflow.yml` gains the agent-less `Plan Approval` step at
position 3 (no `on_enter`, no `on_turn_complete`, `allow_manual_move: true`),
with Implement/Review/Done renumbered to 4/5/6. The Spike and Architect prompts
in both `role-pipeline.workflow.yml` and `spike-design.workflow.yml` now use the
`spike` task document: Spike writes it and never the plan, Architect reads it,
handles absence without inventing evidence, and owns the plan exclusively. The
re-import / manual-move path for an already-imported workflow is recorded in
`docs/fork-evaluation.md`. Config/docs only; verified with
`python3 scripts/list-docs.py validate`.
