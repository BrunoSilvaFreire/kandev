---
status: draft
system: task-cost-ledger
specification_version: 1
migration: in_progress
owners:
  - kandev
---

# Task cost ledger system

## Purpose

The task cost ledger records one priced row per `session.prompt_usage` event and
serves the token and cost figures the Task Chat and Usage panel show.

## Ownership

This system owns the `task_usage_events` schema and its indexes, the ledger
writer that turns a prompt-usage event into a row plus the session rollup, the
`cost_subcents` pricing applied at write time, and the read contracts:
`GET /api/v1/tasks/:id/usage`, `GET /api/v1/tasks/:id/sessions/:sessionId/usage`,
and `GET /api/v1/tasks/:id/usage/breakdown`.

## Exclusions

- Provider quota windows and account-wide usage belong to the [agent
  system](../agents/README.md).
- Office cost dashboards and budgets are an Office surface, not this ledger.
- Reactive provider routing on quota or cost is out of scope; the ledger only
  records what a prompt cost.

## Migration record

Migration is in progress: the legacy `spec.md` still holds the original detail
while new capability documents are added under `requirements/` and
`system-design/`. Use the catalog command to find current sources.

## Related systems

- [Tasks](../tasks/README.md): owns sessions and prompts; this system reads the
  per-prompt usage those produce.
- [Platform](../platform/README.md): owns shared runtime services the ledger
  reads through.
- [Agents](../agents/README.md): owns profiles and providers the ledger rows are
  attributed to.
