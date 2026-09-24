---
status: draft
system: tasks
created: 2026-09-23
owners:
  - brunorbsf
---

# Named Step Transitions Requirements

## Overview

A workflow step already has one ordinary exit: the signal-gated
`on_turn_complete: move_to_next` (ADR 0015). Role workflows need explicit,
named alternatives that an agent chooses from its prompt, such as a forward
handoff from a planner to an implementer, a backward escalation from an
implementer to a planner, and a backward "changes requested" move from a
reviewer. The workflow system owns this contract because the target step and
its one-shot entry options are step configuration.

Named transitions are stored in the existing `workflow_steps.events` JSON under
`events.transitions`; there is no new column and no new trigger. The engine
never evaluates a transition as a trigger. An agent invokes one through
`move_task_kandev(transition=...)`.

## Terminology

- **Named transition:** A step-configured `{name, direction, target, options}`
  entry under `events.transitions`.
- **Direction:** `forward` (target position is greater) or `backward` (target
  position is smaller), checked at invocation time.

## Requirements

### REQ-TASKS-NAMED-TRANSITIONS-001: A step can declare named transitions

**Intent:** Let a workflow author describe the explicit moves an agent may take
without inventing engine triggers.

**User story:** As a workflow author, I want to name the moves out of a step, so
that a prompt can reference them by a stable name.

#### Acceptance criteria

- **AC-TASKS-NAMED-TRANSITIONS-001.1:** When a step is created or updated with
  transitions, the system shall reject an empty name, a name that does not match
  `[a-z0-9_-]+`, a duplicate name, an unknown direction, or a missing target.
- **AC-TASKS-NAMED-TRANSITIONS-001.2:** When a transition names a target step,
  the system shall require the target to be a step in the same workflow.
- **AC-TASKS-NAMED-TRANSITIONS-001.3:** The engine shall not compile a
  transition into any trigger.
- **AC-TASKS-NAMED-TRANSITIONS-001.4:** A portable export shall carry the target
  as `to_step_position`, and an import shall reject a position that matches no
  step; the round trip shall preserve the transition.

### REQ-TASKS-NAMED-TRANSITIONS-002: An agent invokes a transition by name

**Intent:** The agent must be able to reach a named alternative with one call,
and the direction invariant must hold.

**User story:** As a workflow agent, I want to escalate or request changes by
name, so that I do not have to discover step IDs first.

#### Acceptance criteria

- **AC-TASKS-NAMED-TRANSITIONS-002.1:** When `move_task_kandev` is called with
  `transition`, the system shall resolve the target workflow and step from the
  task's current step, and shall not require `workflow_id` or
  `workflow_step_id`.
- **AC-TASKS-NAMED-TRANSITIONS-002.2:** When `move_task_kandev` is called with
  both `transition` and `workflow_step_id`, the system shall reject the call.
- **AC-TASKS-NAMED-TRANSITIONS-002.3:** When the transition name is unknown, the
  system shall reject the call and shall list the available names.
- **AC-TASKS-NAMED-TRANSITIONS-002.4:** When a forward transition targets a step
  at a lower or equal position, or a backward transition a higher or equal
  position, the system shall reject the call.
- **AC-TASKS-NAMED-TRANSITIONS-002.5:** When a transition carries one-shot entry
  options, the system shall merge them with the caller's options: transition
  instructions precede the caller's, and the boolean flags are OR-ed.

## Out of scope

- A UI editor for transitions and kanban transition buttons.
- Transitions in the built-in `apps/backend/config/workflows/*.yml` workflows.
- Engine-level evaluation of transitions as triggers.

## Related specifications

- Legacy move-override contract: `docs/specs/workflow-step-move-overrides/spec.md`.
