# Web reference

Detailed contracts referenced from [AGENTS.md](AGENTS.md).

## Shared Home view model

`lib/view-model/` is the shared filter/grouping model consumed by Kanban/Pipeline, List, Threads, and the sidebar: one `ViewFilterClause` shape, generic `applyViewFilters` (AND across clauses, OR within one) and `applyViewGroup`, plus per-view `SUPPORTED_DIMENSIONS`/`SUPPORTED_GROUPS`. The sidebar and Threads clause types are aliases of the shared shape, and the sidebar `applyFilters` delegates to the shared engine. Repository Groups (a `RepositorySet`) are registered once as the `repositoryGroup` dimension and group key; see [the ADR](../../docs/decisions/2026-09-29-shared-home-view-model.md). Add a new dimension or group key here rather than re-implementing it per view.

The Quick Filter setting is `userSettings.homeQuickFilters` (`HomeViewId -> dimensions`), resolved per view by `resolveQuickFilterDimensions`. Kanban/List persist their active shared `filters`/`group` in `userSettings.taskViewFilters`/`taskViewGroups` (bounded and normalized by the backend); the Kanban board filters through `applyKanbanFilterClauses` when persisted clauses exist and falls back to the legacy repository/priority lens otherwise, and it renders each workflow as group lanes (`components/kanban/workflow-group-lanes.tsx` + `lib/kanban/kanban-grouping.ts`) when a group is active. The List applies the same clauses to its loaded page client-side (`app/tasks/use-list-quick-filter-tasks.ts`); the server still owns pagination. The shared `QuickFilterBar` and `ViewGroupControl` are mounted on the Kanban header and the sidebar filter bar, and the `QuickFilterBar` is also mounted on the List header.

## Tab close contract

A session-tab X and a Quick Chat tab X close the tab; neither deletes. Close removes the dockview panel and records the session in the per-env closed set (`lib/dockview-closed-sessions.ts`) so reconciliation does not reopen it; a Quick Chat close tombstones the local session and removes it from the persisted tab order. Delete is a separate, confirmed context-menu action. `app-sidebar-primary-nav.tsx` places Quick Chats between Usage and New Task.
