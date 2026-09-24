---
status: draft
system: tasks
requirements:
  - REQ-TASKS-NAMED-TRANSITIONS-001
  - REQ-TASKS-NAMED-TRANSITIONS-002
---

# Named Step Transitions System Design

## Purpose and boundaries

The workflow system owns named transitions because they are step configuration
that resolves to a target step and one-shot entry options. The design extends
the existing step-event model and the existing move path; it adds no new
storage, trigger, or engine evaluation.

Adjacent contracts used but not owned: the workflow move-override contract
(`docs/specs/workflow-step-move-overrides/spec.md`) for entry options and the
step-completion signal (ADR 0015) for the ordinary forward exit.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-NAMED-TRANSITIONS-001` | [Data and contracts](#data-and-contracts), [Persistence](#persistence) |
| `REQ-TASKS-NAMED-TRANSITIONS-002` | [Control flow](#control-flow) |

## Components and responsibilities

- **`workflow/models`**: the `StepTransition` shape, `ValidateTransitions`,
  `FindTransition`, and `ValidateTransitionDirection`; reference collection and
  remapping for `ToStepID`; portable conversion to and from `to_step_position`.
- **Workflow controller**: validates transitions and their same-workflow targets
  on step create/update.
- **MCP `move_task_kandev`**: resolves a transition to a concrete workflow and
  step, then continues down the unchanged deferred or immediate move paths.
- **Engine `CompileStep`**: ignores transitions by construction.

## Data and contracts

```go
type StepTransition struct {
    Name           string // unique per step, [a-z0-9_-]+
    Direction      string // "forward" | "backward"
    ToStepID       string // domain target
    ToStepPosition *int   // portable target, export/import only
    Instructions   string
    SkipStepPrompt bool
    ResetContext   bool
}
```

Transitions live under `StepEvents.Transitions` in the existing
`workflow_steps.events` JSON.

## Control flow

1. `move_task_kandev` receives `transition`; it loads the task's current step,
   finds the transition by name, loads the target, and checks the direction.
2. It sets `workflow_id` and `workflow_step_id` from the resolved step and
   target.
3. It normalizes the caller's entry options and merges the transition's options.
4. The existing deferred (active session) or immediate move path runs unchanged.

## Failure and recovery

An unknown name lists the available names. A direction mismatch, a missing
target, and a cross-workflow target are validation errors and change no state.

## Persistence

No schema change: transitions are part of the step's `events` JSON, which the
step-sync equality check already compares byte-for-byte.

## Security

Transition targets are collected with the other step-event references, so a
target outside the workflow is rejected at write time by the same authorization
and same-workflow checks. Invocation re-checks the same-workflow constraint.

## Observability

No new metrics. Resolution failures surface as MCP validation errors.

## Related decisions

- [ADR 0015](../../../decisions/0015-explicit-completion-signal-for-auto-advance.md)
- [Legacy move overrides](../../workflow-step-move-overrides/spec.md)
