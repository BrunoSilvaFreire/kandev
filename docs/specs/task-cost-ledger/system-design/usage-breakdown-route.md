---
status: draft
system: task-cost-ledger
created: 2026-09-23
owners:
  - kandev
requirements:
  - REQ-TASK-COST-LEDGER-BREAKDOWN-001
---

# Per-agent usage breakdown system design

## Purpose and boundaries

This design adds one read-only grouped aggregate and one reusable dockview
panel over the existing task-cost ledger. It owns the
`GET /api/v1/tasks/:id/usage/breakdown` contract and the client-side roll-ups
that turn its groups into the per-agent, per-model and per-session views.

It does not own pricing or the write path. The ledger writer
(`internal/task/usage`) and the existing totals routes stay byte-for-byte
unchanged; this route reads the same rows.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASK-COST-LEDGER-BREAKDOWN-001` | [Grouped read](#grouped-read), [DTO and route](#dto-and-route), [Client roll-ups](#client-roll-ups), [Panel](#panel) |

## Grouped read

`internal/task/repository/sqlite/usage_totals.go` selects one shared aggregate
column list and one shared row reader, used by both the existing single-scope
totals query and the new grouped query. The grouped query is plain portable
`GROUP BY session_id, agent_profile_id, agent_type, model, provider` with
`ORDER BY MAX(occurred_at) DESC`, so it needs no dialect branch.

## DTO and route

`TaskUsageBreakdownDTO` is `{ task_id, task: TaskUsageTotalsDTO(scope "task"),
groups: [ { session_id (nullable), agent_profile_id, agent_type, model,
provider, totals: TaskUsageTotalsDTO(scope "group", scope_id "") } ] }`.
`session_id` serializes to JSON null (never omitted); `groups` is always an
array. The service authorizes and 404s exactly like `GetTaskUsageTotals`, and
the task total comes from that method rather than a Go sum of the groups.

## Client roll-ups

`lib/usage/breakdown.ts` sums the additive fields of groups sharing a view key,
takes the min first and max last timestamp, ANDs `output_tokens_complete`, and
recomputes the hit ratio from the summed tokens (never averages per-group
ratios). The session view merges the task's store sessions so an idle session
still appears.

## Panel

One reusable panel `usage` (`USAGE_PANEL_ID`) is wired exactly like
`prompt-history`: `PANEL_REGISTRY`, `REUSABLE_PANEL_IDS`, `KNOWN_PANEL_IDS`, the
extra-panel action, the "+" menu, the desktop render maps, the layout editor
placeholder, the mobile `MobileSessionCorePanel` union, the mobile "More"
picker, the mobile layout render branch, and the bottom-nav More-entry
condition. The panel fetches on mount and refetches only when one of the task's
sessions reports `promptUsage`; a shared one-minute tick advances the cache
clock without a fetch.

## Cache status

Per session row, cache status uses the shared `cacheStatus` helper keyed on the
**newest usage-ledger event** (`last_event_at`), the only observable provider
round-trip. Messages are ignored entirely: opening or starting a session posts
a message but warms no provider prompt cache, so a fresh message must not turn
the status warm. The session Usage inspector passes
`epochMillisFromWire(session.last_event_at)`; the breakdown row and the
resume-with-handoff offer reuse the same session totals rather than a second
ledger read. It reuses `CACHE_EXPIRY_MS` and `CACHE_RECHECK_MS` with no new
constant, and is labelled an estimate because providers do not expose cache
lifetime.

## Fork refinement (2026-09-25)

The cache definition was previously keyed on the newest message
(`newestMessageAt`); it is now keyed on the newest usage event
(`lastUsageEventAt`). `lib/usage/newest-message.ts` retains only the
`newestMessage` helper used for the resume offer's dismissal key.
EOF
