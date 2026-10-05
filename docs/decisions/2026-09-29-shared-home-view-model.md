# ADR-2026-09-29-shared-home-view-model: One filter and grouping model for Home views

**Status:** accepted
**Date:** 2026-09-29
**Area:** frontend

## Context

Kandev filters task listings through three diverging models: the sidebar view
clause model (`sidebar-view-types.ts`, `filter-dimension-registry.ts`,
`apply-view.ts`), the Threads view model (`thread-view-types.ts`,
`threads-view-filter-registry.ts`), and the ad-hoc Kanban and List display
fields (`kanban-display-dropdown.tsx`, `lib/kanban/filters.ts`,
`tasks-list-options.ts`). Grouping exists only in the sidebar, applies only to
tasks, and is separate from the Threads and Kanban structures.

The requested Quick Filters, Repository Group dimension, and shared grouping
cannot be added to each model independently without repeating the same
definition four times. The repository-group dimension must be defined once and
consumed by Kanban/Pipeline, List, Threads, and the sidebar task navigation.

## Decision

Introduce one shared view model under `apps/web/lib/view-model/`:

- A single `ViewFilterClause` shape (the current sidebar clause: `id`,
  `dimension`, `op`, `value`).
- A `DIMENSIONS` registry that owns each dimension's metadata and its predicate
  over a normalized `ViewItem`.
- `applyViewFilters` with AND-across-clauses and OR-within-clause semantics.
- A `GroupKey` set with `applyViewGroup`, generalized from the sidebar
  `applyGroup` engine and generic over an item accessor.

Each Home view (Kanban/Pipeline, List, Threads, sidebar task navigation)
declares the dimensions and group keys it supports and keeps its own rendering:
Kanban swimlanes, List sections, Threads sections, sidebar headings. Persisted
clause payloads stay compatible because the sidebar and Threads clause types
become aliases of `ViewFilterClause`.

Quick Filter configuration is one user setting,
`home_quick_filters: Record<HomeViewId, Dimension[]>`, while active filter and
grouping values stay in each view's existing persisted preferences.

## Consequences

- Repository Group is registered once and appears in every Home view that
  supports a repository filter.
- Filter and grouping editors become one parameterized component per concern.
- The Kanban and List display preferences migrate from `repositoryIds` and
  priority tokens to `ViewFilterClause[]`; a legacy read path converts the old
  values once and writes the new form.
- Views that cannot express their filter or grouping set in the shared model
  are a stop condition, not a reason to fork the model.
- Plugin task filters registered through `registerTaskFilter` remain separate
  registry entries and are not folded into `DIMENSIONS`.

## Alternatives Considered

- Keep the three models and add a thin shared Quick Filter bar. Rejected: it
  duplicates repository-group semantics and the request explicitly forbids
  per-view implementation.
- Add Repository Group to each model separately. Rejected: four definitions of
  one workspace-level concept drift.
- Move every view onto one shared renderer. Rejected: Kanban, List, Threads,
  and the sidebar have different presentation contracts; only the filter and
  grouping configuration is shared.

## Related records

- [Requirements: Home view filters and grouping](../specs/ui/requirements/home-view-filters-grouping.md).
- [Design: Home view filters and grouping](../specs/ui/system-design/home-view-filters-grouping.md).
- [Requirements: repository sets](../specs/workspaces/requirements/repository-sets.md).
