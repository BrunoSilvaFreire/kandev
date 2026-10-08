# Web reference

Detailed contracts referenced from [AGENTS.md](AGENTS.md).

## Shared Home view model

`lib/view-model/` is the shared filter/grouping model consumed by Kanban/Pipeline, List, Threads, and the sidebar: one `ViewFilterClause` shape, generic `applyViewFilters` (AND across clauses, OR within one) and `applyViewGroup`, plus per-view `SUPPORTED_DIMENSIONS`/`SUPPORTED_GROUPS`. The sidebar and Threads clause types are aliases of the shared shape, and the sidebar `applyFilters` delegates to the shared engine. Repository Groups (a `RepositorySet`) are registered once as the `repositoryGroup` dimension and group key; see [the ADR](../../docs/decisions/2026-09-29-shared-home-view-model.md). Add a new dimension or group key here rather than re-implementing it per view.

The Quick Filter setting is `userSettings.homeQuickFilters` (`HomeViewId -> dimensions`), resolved per view by `resolveQuickFilterDimensions`. Kanban/List persist their active shared `filters`/`group` in `userSettings.taskViewFilters`/`taskViewGroups` (bounded and normalized by the backend); legacy repository and priority preferences migrate to clauses on first use. The Kanban board applies clauses through `applyKanbanFilterClauses` and renders group lanes via `applyViewGroup`/`groupKanbanTasks`. The List applies the same clauses to its loaded page client-side (`app/tasks/use-list-quick-filter-tasks.ts`), and uses shared grouping for native groups while the server still owns pagination. `QuickFilterBar` and `ViewGroupControl` are mounted on Kanban and List, Threads mounts both, and the sidebar filter bar mounts Quick Filters with the shared editor's per-dimension pin toggle.

## Tab close contract

A session-tab X and a Quick Chat tab X close the tab; neither deletes. Close removes the dockview panel and records the session in the per-env closed set (`lib/dockview-closed-sessions.ts`) so reconciliation does not reopen it; a Quick Chat close tombstones the local session and removes it from the persisted tab order. Delete is a separate, confirmed context-menu action. `app-sidebar-primary-nav.tsx` places Quick Chats between Usage and New Task.

## Shared task state

`taskOverview.byId` owns lightweight task records. Board/workflow `taskIds` and sidebar page memberships reference them; compatible `tasks` arrays expose the exact canonical objects. `withTaskOverviewNormalization` merges legacy board, optimistic, and Office writes before publication. Owners cover boards, active detail, displayed pages, and reusable pages; final release evicts the record. Read journals protect live changes/deletions within 1,000 IDs / 1 MiB. Task timestamps and summary revisions are independent. See [shared task state](../../docs/specs/ui/system-design/sidebar-shared-task-state.md) for coverage and reconciliation. Complete `sqlite_nocase_v1` scopes page locally, including sets above 100; other views fetch bounded pages. Covered views have no query or updating announcement. Do not mount sidebar-only all-workflow fetches.

`SidebarTaskPageCache` shares reads per store and retains at most five first pages, 2 MiB including entities, for five minutes from fetch. Later pages remain display-only. Context generations fence reuse. Soft invalidation clears reusable pages but can publish reconciled provisional rows while one trailing refresh recovers membership; provisional pages are never reusable. Summary changes invalidate affected pages. Access denial clears rows and outstanding reads for every consumer. Keep query status in `SidebarTaskQueryStatus`, without duplicate archive errors or layout-shifting banners.
