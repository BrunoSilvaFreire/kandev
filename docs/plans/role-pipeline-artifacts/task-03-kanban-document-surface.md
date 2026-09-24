---
id: task-03
title: Authorized task document routes and Kanban rendering
status: complete
wave: 1
depends_on: []
plan: docs/plans/role-pipeline-artifacts/plan.md
requirements:
  - REQ-TASKS-KANBAN-DOCUMENTS-002
  - REQ-TASKS-KANBAN-DOCUMENTS-003
acceptance_criteria:
  - AC-TASKS-KANBAN-DOCUMENTS-002.1
  - AC-TASKS-KANBAN-DOCUMENTS-002.2
  - AC-TASKS-KANBAN-DOCUMENTS-002.3
  - AC-TASKS-KANBAN-DOCUMENTS-002.4
  - AC-TASKS-KANBAN-DOCUMENTS-003.1
  - AC-TASKS-KANBAN-DOCUMENTS-003.2
  - AC-TASKS-KANBAN-DOCUMENTS-003.3
system_design:
  - docs/specs/tasks/system-design/kanban-task-documents.md
---

# Task 03: Authorized task document routes and Kanban rendering

## Outcome

Task documents are served from `/api/v1/tasks/:id/documents...` with a per-task
authorization guard regardless of the Office feature toggle, and the Kanban plan
panel renders them.

## Scope

In scope: the new mount and its guard middleware in `internal/backendapp`, the
document API base path in the web API client, the `SPIKE` badge, and rendering
`TaskDocuments` in the Kanban plan panel.

Excluded: the Office route group, which keeps its paths and behavior; the
document handler methods themselves, which are reused unchanged; any new
dockview panel or layout default.

## Requirements and design

- `REQ-TASKS-KANBAN-DOCUMENTS-002`, `REQ-TASKS-KANBAN-DOCUMENTS-003`
- [System design](../../specs/tasks/system-design/kanban-task-documents.md)

## Acceptance

- The eight document routes are mounted under `/api/v1` behind a middleware that
  resolves `:id` and calls `Service.AuthorizeTaskAccess`, failing closed; a
  request for an unowned task returns no document data, and the routes are
  present with `features.office` off.
- The Kanban plan panel renders the documents section below the plan, with a
  `SPIKE` badge for `spike`-typed documents and the existing empty state when
  there are none.
- The Office document routes still respond on their current paths.

## Verification

```
cd apps/backend && go test ./internal/backendapp/... -count=1
make -C apps/backend lint
cd apps/web && pnpm vitest run components/task/simple/task-documents.test.tsx components/task/dockview-shared.test.tsx
cd apps/web && pnpm run typecheck && pnpm --filter @kandev/web lint
```

## Likely files and risks

`apps/backend/internal/backendapp/helpers.go`, a new
`apps/backend/internal/backendapp/task_document_routes.go`,
`apps/web/lib/api/domains/office-extended-api.ts`,
`apps/web/components/task/dockview-shared.tsx`,
`apps/web/components/task/simple/task-documents.tsx`.

Risks: the guard is the whole security value of this work order and needs an
explicit denial test; repointing the shared API client base path affects the
Office documents panel too, so its rendering must be re-checked; the `SPIKE`
badge label is a type code, not copy, and follows the existing untranslated
badge table.

## Results

Done. `mountTaskDocumentRoutes` in
`apps/backend/internal/backendapp/task_document_routes.go` mounts the eight
document routes under `/api/v1/tasks/:id/documents...` behind a guard that
fails closed: reads require `workspace.read`, mutations (PUT, DELETE, upload,
restore) require `task.write`, a scope denial is 403 and any other failure 404.
It is wired from `registerTaskRoutes` and is independent of `features.office`.
The shared web client targets the task-scoped path, the `SPIKE` badge is in the
type table, and `PlanContent` renders `TaskDocuments` below the plan panel.
Tests: `internal/backendapp/task_document_routes_test.go` (route presence,
foreign GET 404, foreign PUT 404 with no mutation, unknown 404, unscoped 200,
and a mount-with-Office-off check through the real `registerTaskRoutes`),
`task-documents.test.tsx` (SPIKE badge, lazy content, empty state),
`office-extended-api.documents.test.ts` (paths), and a plan-panel render
assertion in `dockview-shared.test.tsx`. Verified:
`go test ./internal/backendapp/ -count=1`, web vitest on the three files,
`tsc --noEmit`, eslint and prettier on changed files.
