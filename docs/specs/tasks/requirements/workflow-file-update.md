---
status: draft
system: tasks
created: 2026-10-09
owners:
  - kandev
---

# Update an existing workflow from a file

## Overview

A workspace user can apply a local workflow definition to a selected existing workflow after reviewing its effect. This updates the workflow in place and keeps task and session history meaningful.

## Requirements

### REQ-TASKS-WORKFLOW-FILE-001: Confirmed in-place update

**Intent:** Reuse an edited workflow definition without creating a duplicate or losing associations.

- **AC-TASKS-WORKFLOW-FILE-001.1:** The user shall select an existing workflow and a local definition, then see added, removed, and changed steps, order, transitions, configuration, profiles, and instructions before applying it.
- **AC-TASKS-WORKFLOW-FILE-001.2:** Cancel shall leave the workflow unchanged. Confirm shall update the selected workflow with its identity and task associations preserved, or report a validation, ownership, reference, or stale-preview error without a partial update.
- **AC-TASKS-WORKFLOW-FILE-001.3:** A renamed step shall retain its identity only when the definition's stable key or an explicit unambiguous mapping identifies it. An ambiguous rename shall be rejected with a repair path.
- **AC-TASKS-WORKFLOW-FILE-001.4:** A removed step with live task, queue, session, or executable references shall be rejected with affected counts. A safely removed step shall remain identifiable in historical transitions, decisions, sessions, and step visits after restart.
- **AC-TASKS-WORKFLOW-FILE-001.5:** Existing repository-synced workflows shall remain protected from manual updates until the user explicitly releases them from sync. Existing portable definitions shall remain importable.

## Exclusions

- The update does not delete an entire workflow or reassign tasks automatically.
- The update does not reinterpret historical sessions or decisions as belonging to another step.
