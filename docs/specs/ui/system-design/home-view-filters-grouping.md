---
status: draft
system: ui
requirements:
  - REQ-UI-HOME-VIEW-FILTERS-001
  - REQ-UI-HOME-VIEW-FILTERS-002
  - REQ-UI-HOME-VIEW-FILTERS-003
---

# Home View Filters and Grouping System Design

## Purpose and boundaries

The UI system owns the shared client-side view model used by Kanban/Pipeline,
List, Threads, and the sidebar task navigation. It normalizes task projections,
applies filter clauses, and groups the result. It does not own task, workflow,
repository, or session state.

The model replaces the three parallel definitions described in
[ADR-2026-09-29-shared-home-view-model](../../../decisions/2026-09-29-shared-home-view-model.md):
the sidebar model, the Threads model, and the ad-hoc Kanban and List display
fields.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-UI-HOME-VIEW-FILTERS-001` | [Model](#model), [Dimension registry](#dimension-registry), [Migration](#migration) |
| `REQ-UI-HOME-VIEW-FILTERS-002` | [Grouping](#grouping), [Rendering](#rendering) |
| `REQ-UI-HOME-VIEW-FILTERS-003` | [Quick Filters](#quick-filters), [Persistence](#persistence) |

## Model

`apps/web/lib/view-model/` owns:

- `types.ts`: `ViewFilterClause { id, dimension, op, value }`,
  `ViewFilterOp`, `GroupKey`, `ViewItem`, and `HomeViewId`.
- `dimensions.ts`: the `DIMENSIONS` registry. Each entry carries the metadata
  the editors need (`labelKey`, `valueKind`, `ops`, `enumOptions`,
  `defaultOp`, `defaultValue`) and an `evaluate(item, clause)` predicate over a
  normalized `ViewItem`.
- `filters.ts`: `applyViewFilters(items, clauses)`.
- `groups.ts`: `applyViewGroup(items, groupKey, accessor)`.
- `view-capabilities.ts`: each view's `supportedDimensions` and
  `supportedGroups`.

`ViewFilterClause` is the current sidebar clause shape
(`apps/web/lib/state/slices/ui/sidebar-view-types.ts`). The sidebar
`FilterClause` and Threads `ThreadFilterClause` become aliases of it, so
persisted payloads stay compatible. The sidebar registry
(`apps/web/components/task/sidebar-filter/filter-dimension-registry.ts`) and the
Threads registry (`apps/web/components/threads/threads-view-filter-registry.ts`)
move their entries into `DIMENSIONS`; the view-specific dimensions a view does
not support are filtered out through `supportedDimensions`.

## Dimension registry

`DIMENSIONS` includes every dimension either current model supports, including
`archived`, `state`, `workflow`, `workflowStep`, `executorType`, `repository`,
`hasDiff`, `hasPR`, `isPRReview`, `isIssueWatch`, `titleMatch`,
`threadStatus`, `pendingAction`, `taskState`, `primaryAgent`, `priority`,
`blocked`, `hasQueuedPrompts`, `hasActiveSubagents`, `prNeedsAttention`,
`taskType`, `hasActiveError`, `taskLabel`, `taskOrigin`, and
`hasMultipleSessions`. Phase 5 adds `repositoryGroup`.

Plugin task filters registered through `registerTaskFilter` stay outside
`DIMENSIONS` and keep their own rendering in the Kanban display dropdown.

## Grouping

`applyViewGroup` generalizes the sidebar engine
(`apps/web/lib/sidebar/apply-view.ts`). It takes an ordered item list, a
`GroupKey`, and an accessor that returns each item's key and label. It returns
groups plus an explicit ungrouped bucket. Repository grouping keeps the existing
multi-repository combination semantics from
[Sidebar repository grouping](sidebar-repository-grouping.md): the ordered
repository list is the group identity, and incomplete metadata uses the
existing generic multi-repository group.

`GroupKey` gains `priority`. Repository-group grouping follows D7: a task is
placed under the first group in set order that contains its first repository,
and under Ungrouped when none does.

## Migration

- Sidebar and Threads: no persisted change. Editors become one shared
  `ViewFilterEditor` parameterized by `supportedDimensions`, replacing
  `sidebar-filter-popover.tsx` and `threads-view-filter-row.tsx` as the direct
  editors.
- Kanban/Pipeline and List: display preferences gain `filters: ViewFilterClause[]`.
  The existing `repositoryIds` and priority token fields are read once and
  converted to legacy clauses at load, then written back in the shared form.
  This is a clean contract change recorded in `docs/fork-evaluation.md`; no
  dual-write is kept. The Kanban workflow selector stays structural.
- Threads gains a `group` field; List and Kanban gain a `group` display field.

## Rendering

Each view keeps its renderer:

- Kanban/Pipeline renders a non-`none` group as swimlanes.
- List and Threads render groups as sections.
- The sidebar renders groups as headings through the existing group collapse
  model.

One shared `ViewGroupControl` renders the active grouping as a removable chip
in each view header.

## Quick Filters

`home_quick_filters: Record<HomeViewId, Dimension[]>` is a user setting.
`HomeViewId` is `kanban | list | threads | sidebar`. The
`QuickFilterBar` in `apps/web/components/view-model/` reads a view's supported
dimensions, intersects them with the configured list, and renders one inline
control per dimension that reads and writes the same active filter value as the
full filter editor. The `ViewFilterEditor` gains a per-dimension "Show as quick
filter" toggle that writes the setting.

Defaults are `repositoryGroup`, `repository`, `workflow`, and `state`, filtered
to the dimensions each view supports. Unknown view ids and unsupported
dimensions are ignored without dropping valid entries.

## Persistence

Active filters and grouping stay in each view's existing preference store:
sidebar views and draft, Threads views and draft, and the Kanban/List user
display settings (`apps/web/hooks/use-kanban-display-settings.ts`). Only the
Quick Filter configuration is a new user setting; it is added to the backend
user settings DTO and mapped through `apps/web/lib/ssr/user-settings.ts` and the
settings slice.

## Failure and recovery

Invalid stored clauses or group keys are ignored and fall back to the view
default without clearing other valid values. A failed settings write uses the
existing rollback and retry surface for that preference store.

## Verification

- Pure tests for `applyViewFilters`, `applyViewGroup`, and each dimension's
  predicate.
- A test asserts each Home view's `supportedDimensions` and `supportedGroups`
  match its declared capability set.
- Migrated view tests confirm no behavior regression apart from the intended
  legacy-preference conversion.
- Playwright covers filtering and grouping on the sidebar, Threads, Kanban, and
  List.

## Related decisions

- [ADR-2026-09-29-shared-home-view-model](../../../decisions/2026-09-29-shared-home-view-model.md)
- [ADR-2026-08-04-navigation-manifest-boundaries](../../../decisions/2026-08-04-navigation-manifest-boundaries.md)
