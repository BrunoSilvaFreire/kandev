---
status: draft
system: tasks
created: 2026-10-09
owners:
  - kandev
---

# Actionable transition failure diagnostics

## Overview

A rejected or failed workflow transition must tell the acting agent and the user why it did not happen and what can repair it. This includes manual moves, named transitions, deferred moves, and advancement after a completion signal.

## Requirements

### REQ-TASKS-TRANSITION-DIAGNOSTICS-001: Explain every declined transition

**Intent:** Make a failed transition diagnosable and actionable without guessing from a generic error.

- **AC-TASKS-TRANSITION-DIAGNOSTICS-001.1:** Every rejected or failed move or transition shall expose a stable code, a safe human message, and one or more specific reasons naming the failed rule, relevant step or criterion, observed versus required state, and corrective action.
- **AC-TASKS-TRANSITION-DIAGNOSTICS-001.2:** MCP, HTTP, and WebSocket callers shall receive equivalent reasons and remediation for the same failure. Existing error codes shall remain valid. A successful move shall keep its existing response behavior.
- **AC-TASKS-TRANSITION-DIAGNOSTICS-001.3:** A completion-gate refusal shall identify each unmet criterion with description and show the enrolled and current plan identities, gate revision, and the next approval/enrollment or verification action.
- **AC-TASKS-TRANSITION-DIAGNOSTICS-001.4:** A signal or deferred move accepted before execution shall report a later decline durably to the task UI and next agent context without claiming that the transition succeeded.
- **AC-TASKS-TRANSITION-DIAGNOSTICS-001.5:** Every manual move surface shall show the message and expandable reasons with a safe action when available. Desktop and phone users shall reach the same details through accessible controls.
- **AC-TASKS-TRANSITION-DIAGNOSTICS-001.6:** User-repairable failures shall be distinguished from operator/system faults. Inaccessible objects and internal errors shall not disclose protected details or raw stack/database messages.

## Exclusions

- Diagnostics do not override completion criteria, re-enroll plans, retry moves automatically, or change success semantics.
