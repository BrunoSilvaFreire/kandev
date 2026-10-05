---
status: active
system: ui
created: 2026-09-29
owners:
  - kandev
---

# Home View Filters and Grouping Requirements

## Overview

Every Home view filters and groups tasks through one shared view model. The
model defines one clause shape, one dimension registry, one filter evaluator,
and one grouping engine. Each view declares which dimensions and group keys it
supports and keeps its own rendering.

The UI system owns this reusable client-side contract. Task, workspace, and
workflow state stay owned by their systems; the view model only normalizes and
selects them for presentation.

This replaces the three parallel models described in
[the shared view model decision](../../../decisions/2026-09-29-shared-home-view-model.md).

## Terminology

- **Home view:** A task listing surface that supports filtering and grouping:
  Kanban/Pipeline, List, Threads, and the sidebar task navigation.
- **View item:** The normalized task projection a view feeds to the shared
  engine.
- **Dimension:** A filterable task attribute, such as repository, workflow,
  state, priority, or repository group.
- **Filter clause:** One `{ id, dimension, op, value }` predicate.
- **Group key:** A grouping field, such as repository, workflow, state,
  priority, or repository group.
- **Quick Filter:** A dimension configured to appear as an always-visible
  control in a Home view header.

## Requirements

### REQ-UI-HOME-VIEW-FILTERS-001: Shared filter model

**Intent:** One filter definition serves every Home view.

#### Acceptance criteria

- **AC-UI-HOME-VIEW-FILTERS-001.1:** The system shall define one filter clause
  shape and one dimension registry used by Kanban/Pipeline, List, Threads, and
  the sidebar task navigation.
- **AC-UI-HOME-VIEW-FILTERS-001.2:** Multiple clauses shall combine with AND
  semantics; multiple selected values in one clause shall combine with OR
  semantics.
- **AC-UI-HOME-VIEW-FILTERS-001.3:** Each Home view shall declare the dimensions
  it supports. A dimension that a view does not declare shall not be offered or
  applied there.
- **AC-UI-HOME-VIEW-FILTERS-001.4:** Existing persisted sidebar and Threads
  clauses shall load unchanged. Their clause type shall be the shared clause
  shape.
- **AC-UI-HOME-VIEW-FILTERS-001.5:** Kanban/Pipeline and List shall express
  repository and priority filtering as shared clauses. Existing
  `repository_ids` and priority display values shall be read once as legacy
  clauses and written back in the shared form.
- **AC-UI-HOME-VIEW-FILTERS-001.6:** Plugin task filters registered through the
  existing task-filter registration shall remain separate registry entries and
  shall keep their current behavior.
- **AC-UI-HOME-VIEW-FILTERS-001.7:** The existing filter editors shall become
  one parameterized filter editor over the supported dimensions. Clear, add,
  and remove behavior shall be preserved for each migrated view.

### REQ-UI-HOME-VIEW-FILTERS-002: Shared grouping model

**Intent:** Grouping has one definition and one visual contract per view.

#### Acceptance criteria

- **AC-UI-HOME-VIEW-FILTERS-002.1:** The system shall define one group-key set
  and one grouping engine used by every Home view that supports grouping.
- **AC-UI-HOME-VIEW-FILTERS-002.2:** Group keys shall include repository group,
  repository, workflow, state, and priority, and may include view-specific keys.
  A view shall only offer the keys it declares.
- **AC-UI-HOME-VIEW-FILTERS-002.3:** Grouping shall be independent from
  filtering: filtering selects the visible set and grouping organizes that set.
- **AC-UI-HOME-VIEW-FILTERS-002.4:** The active grouping shall be visible in
  the view header and removable in one action through one shared grouping
  control.
- **AC-UI-HOME-VIEW-FILTERS-002.5:** Kanban/Pipeline shall render a non-none
  grouping as swimlanes. List and Threads shall render groups as sections. The
  sidebar shall render groups as headings. The grouping configuration and its
  semantics shall come from the shared engine.
- **AC-UI-HOME-VIEW-FILTERS-002.6:** Group order and ungrouped items shall be
  deterministic. Items with no value for the active key shall appear in an
  explicit ungrouped group.
- **AC-UI-HOME-VIEW-FILTERS-002.7:** Grouping preferences shall persist with the
  view's existing display or saved-view preferences.

### REQ-UI-HOME-VIEW-FILTERS-003: Quick Filters

**Intent:** Frequently used filters are reachable without opening the full
filter menu.

#### Acceptance criteria

- **AC-UI-HOME-VIEW-FILTERS-003.1:** Each supported dimension shall be
  configurable to appear as a Quick Filter in its Home view header. The full
  filter menu shall remain available.
- **AC-UI-HOME-VIEW-FILTERS-003.2:** The Quick Filter configuration shall be one
  user setting keyed by Home view id. It shall persist across reloads and apply
  in another signed-in client.
- **AC-UI-HOME-VIEW-FILTERS-003.3:** A Quick Filter control shall read and write
  the same active filter value as the full filter menu, so the two stay in
  sync.
- **AC-UI-HOME-VIEW-FILTERS-003.4:** Only dimensions supported by the view shall
  be offered as Quick Filters there.
- **AC-UI-HOME-VIEW-FILTERS-003.5:** The default Quick Filter configuration
  shall include repository group, repository, workflow, and state where the
  view supports them.
- **AC-UI-HOME-VIEW-FILTERS-003.6:** An unknown view id or an unsupported
  dimension in the stored configuration shall be ignored without error, without
  dropping the valid entries.
- **AC-UI-HOME-VIEW-FILTERS-003.7:** Repository Group shall be available as both
  a normal filter and a Quick Filter in every view that supports a repository
  filter.

## Out of scope

- A shared renderer for the views themselves.
- Server-side task-query execution or a new task-query endpoint.
- Changing task, workflow, repository, or session ownership.
- Reimplementing plugin task filters inside the shared dimension registry.
