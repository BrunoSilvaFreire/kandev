---
status: draft
system: task-cost-ledger
created: 2026-09-23
owners:
  - kandev
---

# Per-agent usage breakdown requirements

## Overview

The task-cost ledger already records one priced row per `session.prompt_usage`
event with the session, agent profile, agent type, model and provider. The two
existing read routes return only two coarse totals (task and session), so the
Task Chat cannot show which agent, model or provider spent the tokens and cost.
This capability adds one grouped read over the same ledger so the Usage panel
can break a task down per agent, per model and per session. It is read-only and
adds no pricing, no schema, and no write path.

## Terminology

- **Group**: the finest ledger grain, the tuple (session, agent profile, agent
  type, model, provider).
- **Roll-up**: a client-side sum of groups for one view (agent, model, session).

## Requirements

### REQ-TASK-COST-LEDGER-BREAKDOWN-001: Grouped usage breakdown

**Intent:** Let a user see per-agent, per-model and per-session token and cost
use for a task without a second price table or a new aggregation contract.

**User story:** As a task owner, I want a per-agent cost breakdown for a task,
so that I can see which agent, model or provider is consuming tokens and budget.

#### Acceptance criteria

- **AC-TASK-COST-LEDGER-BREAKDOWN-001.1:** When the breakdown route is called
  for an existing task, the system shall return the task total plus one group
  per (session, agent profile, agent type, model, provider) tuple, each group's
  totals shaped exactly like the existing totals DTO.
- **AC-TASK-COST-LEDGER-BREAKDOWN-001.2:** When a contributing row's session was
  deleted, the system shall report that group's `session_id` as JSON null, never
  omit the field.
- **AC-TASK-COST-LEDGER-BREAKDOWN-001.3:** When a task has no usage rows, the
  system shall return HTTP 200 with the zeroed task total and an empty `groups`
  array (never null).
- **AC-TASK-COST-LEDGER-BREAKDOWN-001.4:** When the task is unknown or not
  visible to the caller, the system shall return the same not-found response as
  the existing totals routes.
- **AC-TASK-COST-LEDGER-BREAKDOWN-001.5:** The system shall order groups by
  their newest contributing row, descending, so the most recently active group
  is first.
- **AC-TASK-COST-LEDGER-BREAKDOWN-001.6:** The system shall compute the task
  total from the existing task-total query, not by summing the groups, so an
  append between the two reads can put the total at most one prompt ahead and
  the next read reconciles it.

## Out of scope

- Any new price table or client-side pricing. The breakdown reuses the persisted
  `cost_subcents` the ledger already writes.
- Any write path; the ledger writer is unchanged.
- A per-session route for other tasks or a cross-task aggregate.
