---
status: draft
system: tasks
created: 2026-09-23
owners:
  - brunorbsf
---

# Approval Requests Requirements

## Overview

A workflow role that owns a durable artifact (the task plan or a task document)
must be able to ask a human to approve it, revise it, or reject it, and stay
blocked until the human answers. The tasks system owns this contract because the
subject and the pending plan comments are task records, and because the request
must block the agent's own turn.

Approval reuses the clarification pipeline rather than adding a second
question store: the bundle is one fixed question whose option IDs are the
stable decisions `approve`, `revise`, and `reject`, plus approval metadata.
Existing clarification chat rendering, the Needs-you Inbox, the blocking wait
with keep-alive, the timeout fallback, and persistence all apply unchanged.

## Terminology

- **Approval request:** A clarification bundle carrying `approval` metadata and
  exactly one fixed question.
- **Subject:** What is being approved: `task_plan` or `document`.
- **Plan comments:** Pending feedback rows attached to the current task plan.
- **Subject edited:** The subject's current version differs from the version
  recorded when the request was created.

## Requirements

### REQ-TASKS-APPROVAL-REQUESTS-001: A session can request approval and stay blocked

**Intent:** Let an agent hand a durable artifact to a human without ending its
turn or inventing a second waiting mechanism.

**User story:** As a workflow agent, I want to request approval and wait, so
that I can act on the human's decision in the same turn.

#### Acceptance criteria

- **AC-TASKS-APPROVAL-REQUESTS-001.1:** When an agent calls
  `request_approval_kandev`, the system shall create a clarification bundle with
  exactly one question whose option IDs are `approve`, `revise`, and `reject`,
  and shall persist the approval metadata with the bundle's messages.
- **AC-TASKS-APPROVAL-REQUESTS-001.2:** When the request is created, the system
  shall record the subject's current version, resolved from the task, never from
  the caller.
- **AC-TASKS-APPROVAL-REQUESTS-001.3:** When the user answers, the system shall
  return the decision, the feedback text, the rendered plan comments, whether the
  subject was edited, and the subject's current version.
- **AC-TASKS-APPROVAL-REQUESTS-001.4:** When the agent's tool call times out or
  is cancelled, the system shall deliver the same approval content through the
  existing timeout-fallback resume path.

### REQ-TASKS-APPROVAL-REQUESTS-002: Revise carries plan comments

**Intent:** A revise decision must be able to reference the exact pending plan
comments the user annotated, without a second store.

**User story:** As a task owner, I want to comment on the plan and click Revise,
so that the agent receives every comment and does not lose the one I already
wrote.

#### Acceptance criteria

- **AC-TASKS-APPROVAL-REQUESTS-002.1:** When a revise answer carries plan
  comment references, the system shall validate them against the task's current
  pending comments before claiming the bundle, and shall reject stale or foreign
  references as an answer-validation error.
- **AC-TASKS-APPROVAL-REQUESTS-002.2:** When a revise answer carries references,
  the system shall render them with the existing plan-comment formatter into the
  answer.
- **AC-TASKS-APPROVAL-REQUESTS-002.3:** When the answer is delivered, the system
  shall consume the referenced comments best-effort; a consumption failure shall
  leave them pending and shall not fail the delivered answer.
- **AC-TASKS-APPROVAL-REQUESTS-002.4:** When an `approve` or `reject` answer
  carries plan comment references, the system shall reject the answer.
- **AC-TASKS-APPROVAL-REQUESTS-002.5:** When the subject is not a task plan, the
  system shall reject plan comment references.

### REQ-TASKS-APPROVAL-REQUESTS-003: The user may edit the subject directly

**Intent:** The user must be able to correct the artifact in place, and the
agent must not overwrite that edit.

**User story:** As a task owner, I want to edit the plan and answer Revise, so
that the agent continues from my edit instead of clobbering it.

#### Acceptance criteria

- **AC-TASKS-APPROVAL-REQUESTS-003.1:** When the user edits the subject between
  the request and the answer, the system shall report `subject_edited` as true
  and shall return the subject's current version.
- **AC-TASKS-APPROVAL-REQUESTS-003.2:** When a revise answer has no feedback, no
  plan comment references, and no subject edit, the system shall reject it.
- **AC-TASKS-APPROVAL-REQUESTS-003.3:** When the user dismisses the request, the
  system shall record the decision `reject`.

## Out of scope

- Inbox-row approve/reject quick actions; the answer happens on the task page.
- Plan comments on non-plan documents.
- An engine-enforced "first plan needs approval" rule; it stays prompt-level.
