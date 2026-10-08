---
status: draft
system: tasks
created: 2026-10-05
owners:
  - kandev
---

# Plan Increment Completion Gate and Possible Question Hint Requirements

## Overview

Role Pipeline tasks execute multi-step implementation workflows divided into discrete increments.
Previously, completion gates evaluated only binary task state or external pull requests. A review
agent could invoke a terminal `pass` transition after verifying only an initial increment (for example,
`I1`), causing the task to enter `Done` while subsequent planned increments (such as `I2` through `I14`)
remained pending and unverified.

Furthermore, agents in autonomous workflows frequently end decision turns in conversational prose
without invoking a structured interaction barrier (`ask_user_question_kandev` or `request_approval_kandev`).
Because conversational prose does not create a structured clarification request, tasks settle into
`WAITING_FOR_INPUT` with an empty `pending_action`, rendering them invisible in operator triage queues.

This specification establishes:
1. Enrolled plan increments as typed task completion criteria using a first-class `plan_increment`
   evidence subject.
2. Independent increment verification that survives subsequent task moves, implementation turns,
   and unrelated task metadata updates.
3. Terminal completion gating that blocks premature `pass` transitions while increments remain
   unverified, preserving non-terminal review and audited human overrides.
4. A non-authoritative `possible_question` projection for sessions that settle to `WAITING_FOR_INPUT`
   with an unprompted trailing question and no active interaction barrier.

## Terminology

- **Plan increment:** A discrete, named unit of work declared in an approved task plan (for example,
  `I1`, `I2`), containing a stable identifier and a description of its delivery scope.
- **Enrolled criterion:** A `TaskCompletionCriterion` created from a plan increment, tracked in the
  task's completion criteria set with a repository-assigned `CriterionRevision`.
- **`plan_increment` evidence subject:** A typed `TaskCompletionEvidenceSubject` whose `Kind` is
  `plan_increment`, whose `ID` is the stable increment ID, and whose `Revision` is the decimal string
  representation of the criterion's repository-issued `CriterionRevision`.
- **Independent verification:** Verification of one increment that remains valid across subsequent
  task updates and moves, invalidating only when that specific increment's definition is altered.
- **Terminal transition:** A workflow step transition into a step configured with
  `complete_task_on_enter: true` (such as `Done`).
- **`possible_question` hint:** A non-authoritative advisory indicator on the task status summary
  signaling that an agent turn ended with a trailing question or decision cue without registering a
  structured interaction barrier.

## Requirements

### REQ-TASKS-PLAN-INCREMENT-001: Typed plan increment enrollment

**Intent:** Bind declared plan increments to versioned task completion criteria under an approved plan revision.

#### Acceptance criteria

- **AC-TASKS-PLAN-INCREMENT-001.1:** When an approved task plan declares increments, the system shall
  support enrolling the increments as typed `TaskCompletionCriterion` records in the task's completion set.
- **AC-TASKS-PLAN-INCREMENT-001.2:** Enrollment shall accept stable increment identifiers and descriptions,
  binding them to an immutable `TaskPlanRevision` identity in a single database transaction.
- **AC-TASKS-PLAN-INCREMENT-001.3:** Unenrolled tasks and legacy tasks with no completion criteria shall
  retain existing unblocked completion behavior.
- **AC-TASKS-PLAN-INCREMENT-001.4:** The system shall reject criterion enrollment or modification requested
  by an agent unless accompanied by an approved plan revision receipt or confirmed by an authenticated human.
- **AC-TASKS-PLAN-INCREMENT-001.5:** When a subsequent plan revision updates enrolled increments, the system
  shall preserve existing criterion revisions and verification evidence for unchanged increments, bump the
  criterion revision and invalidate evidence for modified increments, and require explicit human confirmation
  to remove or weaken any unmet increment.

### REQ-TASKS-PLAN-INCREMENT-002: First-class plan_increment evidence subject

**Intent:** Provide an immutable evidence subject that verifies a specific increment revision independently of task updates.

#### Acceptance criteria

- **AC-TASKS-PLAN-INCREMENT-002.1:** The system shall accept `plan_increment` as a first-class
  `TaskCompletionEvidenceSubject` kind across models, repositories, and service validation.
- **AC-TASKS-PLAN-INCREMENT-002.2:** A `plan_increment` evidence subject shall declare the stable increment
  identifier as its `ID` and the repository-issued integer `CriterionRevision` as its string `Revision`.
- **AC-TASKS-PLAN-INCREMENT-002.3:** When verifying a `plan_increment` criterion, the system shall require
  that the evidence subject kind is `plan_increment`, the subject ID matches the criterion ID, and the
  evidence revision equals the criterion's current repository-issued `CriterionRevision`.
- **AC-TASKS-PLAN-INCREMENT-002.4:** Unrelated task updates, workflow step changes, and verifications of sibling
  criteria on the same task shall not invalidate an existing verified `plan_increment` criterion.
- **AC-TASKS-PLAN-INCREMENT-002.5:** Modifying an increment's description or required subject shall increment
  its `CriterionRevision`, causing existing verification with the prior revision to be evaluated as stale.

### REQ-TASKS-PLAN-INCREMENT-003: Terminal completion gate and review pass enforcement

**Intent:** Prevent premature completion while increments remain pending, while preserving partial review cycles.

#### Acceptance criteria

- **AC-TASKS-PLAN-INCREMENT-003.1:** The task completion guard shall block any transition into a completing
  workflow step (including named `pass` transitions to `Done`) while one or more enrolled criteria remain
  unverified or carry stale evidence.
- **AC-TASKS-PLAN-INCREMENT-003.2:** A Review agent shall be able to verify one completed increment and execute
  a non-terminal transition (such as `request_changes` returning to Implement) while subsequent increments
  remain pending.
- **AC-TASKS-PLAN-INCREMENT-003.3:** When a transition into a completing step is blocked by unverified criteria,
  the MCP move tool shall return a structured validation error containing the blocked criterion IDs, descriptions,
  current workflow step, available non-terminal transitions, and guidance for human override.
- **AC-TASKS-PLAN-INCREMENT-003.4:** An authenticated human operator shall be able to override unverified completion
  criteria using a recorded `TaskCompletionMoveOverride` with a mandatory reason, persisting an audited history entry.

### REQ-TASKS-PLAN-INCREMENT-004: Non-authoritative possible-question projection

**Intent:** Surface sessions waiting on human decisions in prose without fabricating false clarification requests.

#### Acceptance criteria

- **AC-TASKS-PLAN-INCREMENT-004.1:** When a task session settles to `WAITING_FOR_INPUT` with `pending_action == ""` (null),
  no in-turn workflow step transition, no completion signal, and a trailing agent prose message containing a decision
  or question cue, the status summary shall project a `possible_question` hint.
- **AC-TASKS-PLAN-INCREMENT-004.2:** The `possible_question` hint shall not generate a synthetic clarification bundle,
  shall not halt background execution, and shall not block auto-advance transitions.
- **AC-TASKS-PLAN-INCREMENT-004.3:** Triage ranking shall prioritize active permissions and structured clarifications
  first, followed by `parked_on_background_work`, followed by `possible_question` hints.
- **AC-TASKS-PLAN-INCREMENT-004.4:** The system shall clear the `possible_question` hint immediately upon receipt
  of a user message, a new agent turn, a workflow step move, or session completion.

### REQ-TASKS-PLAN-INCREMENT-005: Durable plan approval receipt and fail-closed binding

**Intent:** Establish an immutable, repository-owned approval receipt for plan revisions and enforce fail-closed gate evaluation when plans are modified after enrollment.

#### Acceptance criteria

- **AC-TASKS-PLAN-INCREMENT-005.1:** When an approval request is created with subject `task_plan`, the system shall record the target `plan_revision_id` and the current plan HEAD `write_version` within the request's durable metadata.
- **AC-TASKS-PLAN-INCREMENT-005.2:** The system shall persist an immutable `TaskPlanApprovalReceipt` in the database only when the winning human answer claims `decision = "approve"` with `subject_edited = false` and matching HEAD `write_version` within the same transaction as the clarification bundle claim.
- **AC-TASKS-PLAN-INCREMENT-005.3:** An approval response resulting in `revise`, `reject`, `subject_edited = true`, or a version mismatch shall not persist an approval receipt and shall not permit agent criteria enrollment.
- **AC-TASKS-PLAN-INCREMENT-005.4:** When an agent enrolls criteria for a plan revision, the system shall verify in a single transaction that a valid approval receipt exists for the specified `(task_id, plan_revision_id, write_version)` matching both current plan HEAD and revision metadata.
- **AC-TASKS-PLAN-INCREMENT-005.5:** Any subsequent plan write shall advance the plan HEAD `write_version`, rendering the existing criteria enrollment binding stale.
- **AC-TASKS-PLAN-INCREMENT-005.6:** While a task plan binding is stale or unenrolled on a scoped pipeline, the completion gate shall fail closed, blocking terminal transitions and Architect-to-Implement handoffs until explicit re-approval and criteria reconciliation occur.
