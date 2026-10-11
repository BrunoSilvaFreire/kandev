---
status: draft
system: tasks
created: 2026-10-06
owners:
  - kandev
---

# Role Pipeline Architect Technical Design Artifact

## Overview

The Role Pipeline Architect currently writes a task plan, but the plan does not consistently give the Implementer and Reviewer enough technical direction. Each Architect pass that creates or materially changes an implementation plan needs a separate, task-scoped technical design. The task plan remains a short ordered delivery outline.

## Requirements

### REQ-TASKS-ROLE-DESIGN-001: Durable technical design

**Intent:** Give the Implementer and Reviewer a concrete design that survives workflow transitions.

#### Acceptance criteria

- **AC-TASKS-ROLE-DESIGN-001.1:** Before requesting approval of a new or materially revised plan, the Architect shall save a Markdown technical design file in the repository and publish the same content as a task document with key `technical-design`.
- **AC-TASKS-ROLE-DESIGN-001.2:** The design shall state the verified baseline, technical decisions and rationale, component and data boundaries, control flow, failure behavior, example code or contract shapes, coding guidance, and focused verification. Unknowns shall be explicit rather than invented.
- **AC-TASKS-ROLE-DESIGN-001.3:** The task plan shall remain a high-level Markdown outline and link both the repository design path and task document key. Implement and Review shall read the linked design before acting.
- **AC-TASKS-ROLE-DESIGN-001.4:** A material design change shall update the file and task document together, then revise the plan references before approval or handoff. A missing or divergent copy shall stop the Architect handoff.
- **AC-TASKS-ROLE-DESIGN-001.5:** The technical design shall not replace the task plan, typed completion criteria, requirement and system-design specifications, or the applicable human or audited-minor plan approval decision.
- **AC-TASKS-ROLE-DESIGN-001.6:** Before the first Implement handoff, the Architect shall check the evidence needed for every planned increment: owning requirements and design, schema/data ownership, current writer and reader paths, migration and concurrency boundary, verification environment, and unresolved product decisions. The handoff shall name remaining assumptions and where to resume; missing material evidence shall trigger exploration or human decision rather than an implementation-time guess.
- **AC-TASKS-ROLE-DESIGN-001.7:** After a human-approved baseline, an Architect may self-approve a narrowly scoped plan clarification only when declared completion criteria and their evidence subjects are unchanged and no product definition, public API, persistence/migration, authorization/security, destructive action, dependency order, or material risk changes. The system shall bind an auditable minor-revision receipt to the exact plan revision and write version; a prompt claim alone is not approval authority. The system shall accept a minor revision only when its content is the human-approved baseline followed by an appended, non-normative advisory clarification section, and only from the session running in the workflow step that obtained the baseline human approval. All other revisions require human approval.
- **AC-TASKS-ROLE-DESIGN-001.8:** A self-approved minor revision shall rebind unchanged completion criteria to its exact plan head without losing verification evidence. The Architect-to-Implement handoff shall not be blocked merely because implementation criteria are not yet verified; terminal completion remains gated on their verification. A stale or unenrolled plan still blocks handoff.

## Exclusions

- Automatic generation of code, tests, or schema migrations from the document.
- A new task-document type or a second plan approval mechanism.
