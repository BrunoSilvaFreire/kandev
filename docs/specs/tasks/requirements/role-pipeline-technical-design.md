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
- **AC-TASKS-ROLE-DESIGN-001.5:** The technical design shall not replace the task plan, typed completion criteria, requirement and system-design specifications, or the human plan approval decision.

## Exclusions

- Automatic generation of code, tests, or schema migrations from the document.
- A new task-document type or a second plan approval mechanism.
