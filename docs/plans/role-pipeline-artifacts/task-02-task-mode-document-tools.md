---
id: task-02
title: Task-mode document MCP tools
status: complete
wave: 1
depends_on: []
plan: docs/plans/role-pipeline-artifacts/plan.md
requirements:
  - REQ-TASKS-KANBAN-DOCUMENTS-001
acceptance_criteria:
  - AC-TASKS-KANBAN-DOCUMENTS-001.1
  - AC-TASKS-KANBAN-DOCUMENTS-001.2
  - AC-TASKS-KANBAN-DOCUMENTS-001.3
  - AC-TASKS-KANBAN-DOCUMENTS-001.4
system_design:
  - docs/specs/tasks/system-design/kanban-task-documents.md
---

# Task 02: Task-mode document MCP tools

## Outcome

A Kanban task session can list, read, and write task documents, defaulting to
its own task, with the existing relationship access rules intact.

## Scope

In scope: the `task-documents` profile entry in `internal/mcp/server/server.go`,
the three tool declarations and handlers in
`internal/mcp/server/handoff_handlers.go`, and their tests.

Excluded: the handoff service's access rules, the storage layer, the HTTP
surface, and any new document type at the storage layer.

## Requirements and design

- `REQ-TASKS-KANBAN-DOCUMENTS-001`
- [System design](../../specs/tasks/system-design/kanban-task-documents.md)

## Acceptance

- The three document tools are registered when the MCP profile is kanban or
  office, and remain absent in the other modes.
- `task_id` is optional on all three tools; omitted or `self` resolves to the
  calling session's task, and an empty resolution returns a clear error rather
  than a blank lookup.
- A write targeting an unrelated task returns `access_denied`, and a write to
  self leaves the task plan untouched.

## Verification

```
cd apps/backend && go test ./internal/mcp/... -count=1
make -C apps/backend lint
```

## Likely files and risks

`apps/backend/internal/mcp/server/server.go`,
`apps/backend/internal/mcp/server/handoff_handlers.go`,
`apps/backend/internal/mcp/server/handoff_handlers_test.go`.

Risk: the tool-list completeness tests enumerate per-mode tool sets; the mode
expectations must be updated in the same change or the suite fails for the right
reason.

## Results

Done. The `task-documents` profile group is enabled for kanban and office;
`task_id` is optional on all three tools and defaults to the calling task, with
a clear error when no task context exists. The tool-count completeness tests in
`internal/mcp/server/server_test.go` were updated (task mode 42 → 45). Tests
added in `handoff_handlers_test.go` cover the self default, explicit-target
forwarding, the empty-context error, an access-denied backend response, and the
optional `task_id` schema. Verified:
`go test ./internal/mcp/... -count=1`, `go vet`, `gofmt`.
