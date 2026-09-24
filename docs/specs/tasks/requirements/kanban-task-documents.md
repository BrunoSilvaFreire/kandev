---
status: draft
system: tasks
created: 2026-09-22
owners:
  - brunorbsf
---

# Kanban Task Documents Requirements

## Overview

Task documents (`task_documents`, keyed per task, with revisions) already exist
and are already the durable artifact store used for parent/child coordination.
Today they are reachable only from Office: the MCP document tools are registered
for Office sessions only, and the HTTP routes are mounted under `/api/v1/office`
behind the `features.office` gate.

A Kanban workflow that splits roles across steps needs the same durable
artifact. A cheap investigation step must leave a report that a later step can
read, without overwriting the task implementation plan, which the planning step
owns. The tasks system owns the contract because `task_documents` is a task
record, not an Office record; Office is one consumer.

## Terminology

- **Task document:** A keyed, revisioned markdown record attached to a task.
- **Task mode:** An MCP session bound to a Kanban task (`ModeTask`), as opposed
  to an Office session (`ModeOffice`).
- **Spike report:** A task document with key `spike`, written by an
  investigation step and read by a later planning step.

## Requirements

### REQ-TASKS-KANBAN-DOCUMENTS-001: Task-mode agents read and write task documents

**Intent:** Let a Kanban workflow step persist a durable artifact that a later
step reads, without competing for the single task plan.

**User story:** As a workflow author, I want each role step to own its own
artifact, so that an investigation report and an implementation plan can both
exist and neither overwrites the other.

#### Acceptance criteria

- **AC-TASKS-KANBAN-DOCUMENTS-001.1:** When an agent session is in task mode,
  the system shall expose the list, get, and write task-document tools.
- **AC-TASKS-KANBAN-DOCUMENTS-001.2:** When a document tool is called with
  `task_id` omitted or equal to `self`, the system shall resolve it to the
  calling session's task.
- **AC-TASKS-KANBAN-DOCUMENTS-001.3:** When a document tool targets a task the
  caller may not reach, the system shall return `access_denied` and shall not
  disclose the document's existence or content. The existing relationship rules
  are unchanged: read for self, ancestors, descendants, and siblings sharing a
  non-empty parent; write for self and ancestors only.
- **AC-TASKS-KANBAN-DOCUMENTS-001.4:** When an agent writes a document, the
  system shall preserve the task plan unchanged.

### REQ-TASKS-KANBAN-DOCUMENTS-002: Task documents are visible in the Kanban task view

**Intent:** An artifact a human cannot see is not a deliverable. The report must
be readable where the task is worked.

**User story:** As a task owner, I want to read the documents an agent attached
to my Kanban task, so that I can check an investigation before approving the
work it feeds.

#### Acceptance criteria

- **AC-TASKS-KANBAN-DOCUMENTS-002.1:** When a user opens the Kanban task plan
  panel, the system shall list that task's documents with type badge, title, and
  relative updated time, and shall allow expanding one to read its content.
- **AC-TASKS-KANBAN-DOCUMENTS-002.2:** When a document's type is `spike`, the
  system shall render a distinct `SPIKE` badge.
- **AC-TASKS-KANBAN-DOCUMENTS-002.3:** When the task has no documents, the
  system shall render the empty state and shall not render an error.
- **AC-TASKS-KANBAN-DOCUMENTS-002.4:** When the `features.office` toggle is off,
  the system shall still list and render Kanban task documents.

### REQ-TASKS-KANBAN-DOCUMENTS-003: Non-Office document HTTP access is authorized per task

**Intent:** Moving documents out from behind the Office route group must not
lose the ownership check that group provided.

#### Acceptance criteria

- **AC-TASKS-KANBAN-DOCUMENTS-003.1:** When a browser caller requests a document
  route for a task it does not own, the system shall reject the request without
  returning document data.
- **AC-TASKS-KANBAN-DOCUMENTS-003.2:** When `features.office` is off, the system
  shall serve the task-scoped document routes.
- **AC-TASKS-KANBAN-DOCUMENTS-003.3:** The existing `/api/v1/office` document
  routes shall keep their current paths and behavior.

### REQ-TASKS-KANBAN-DOCUMENTS-004: Superseded artifacts stay addressable

**Intent:** A replacement plan or a second investigation must not erase the one
before it; a later step or a human must still be able to open it.

**User story:** As a workflow author, I want an escalation-driven replan to keep
the previous plan readable, so that the Implementer gets the new plan while the
earlier one remains available for comparison and audit.

#### Acceptance criteria

- **AC-TASKS-KANBAN-DOCUMENTS-004.1:** When an agent writes a task document
  under a key that no document of the task uses, the system shall create a
  separate document, whatever its type, leaving other documents of the same
  type unchanged.
- **AC-TASKS-KANBAN-DOCUMENTS-004.2:** When an agent updates the task plan with
  `new_revision` set, the system shall record the write as a new plan revision
  even when it would otherwise coalesce into the latest one, leaving the prior
  revision's content unchanged and addressable by revision number.
- **AC-TASKS-KANBAN-DOCUMENTS-004.3:** When `new_revision` is absent, the system
  shall keep the existing plan-revision coalescing behavior.

### REQ-TASKS-KANBAN-DOCUMENTS-005: Conditional Plan Approval gate

**Intent:** The first implementation plan always requires human sign-off, while
escalation replanning returns straight to implementation unless human
review is explicitly requested.

**User story:** As a project lead, I want initial plans vetted by a person before
work starts, but routine escalation advice or minor replans to reach the implementer
without getting stuck at an approval gate.

#### Acceptance criteria

- **AC-TASKS-KANBAN-DOCUMENTS-005.1:** When the planning step finishes its initial
  turn, the task shall advance to the Plan Approval gate step.
- **AC-TASKS-KANBAN-DOCUMENTS-005.2:** After an escalation from the implementer,
  the planning step shall return the task directly to the implementer step by
  default without entering the Plan Approval gate.
- **AC-TASKS-KANBAN-DOCUMENTS-005.3:** When an escalation replan requires human
  sign-off, the planning step shall signal completion with a summary starting with
  `APPROVAL REQUESTED:`, parking the task at the Plan Approval gate.
- **AC-TASKS-KANBAN-DOCUMENTS-005.4:** The conditional gate routing rules shall be
  enforced by step prompts and workflow configuration rather than engine-level
  hard constraints.
- **AC-TASKS-KANBAN-DOCUMENTS-005.5:** When parking at the Plan Approval gate, the
  planning step shall record its implementer-facing handoff or advice in the plan's
  top revision note, ensuring the handoff context is preserved across the agent-less
  gate step.

## Out of scope

- Prompt wording for any specific workflow outside the Role Pipeline example.
- The ADR 0015 `manual_fallback` control and its counter, tracked separately.
- Any change to task plan semantics or ownership beyond the opt-in
  `new_revision` flag.
- Document editing from the Kanban board or list views.
