---
status: draft
system: tasks
requirements:
  - REQ-TASKS-KANBAN-DOCUMENTS-001
  - REQ-TASKS-KANBAN-DOCUMENTS-002
  - REQ-TASKS-KANBAN-DOCUMENTS-003
---

# Kanban Task Documents System Design

## Purpose and boundaries

`task_documents` and `DocumentService` already live in `internal/task`. Office
owns neither; it owns a route group and an MCP profile that happen to be the
only current consumers. This design widens the two consumer surfaces and adds
nothing to the storage or service layer.

Adjacent contracts used but not owned here: the Office route group and its
workspace scope middleware, the MCP profile registry, and the dockview panel
registry.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-KANBAN-DOCUMENTS-001` | [MCP surface](#mcp-surface) |
| `REQ-TASKS-KANBAN-DOCUMENTS-002` | [Frontend](#frontend) |
| `REQ-TASKS-KANBAN-DOCUMENTS-003` | [HTTP surface](#http-surface) |
| `REQ-TASKS-KANBAN-DOCUMENTS-004` | [Data and contracts](#data-and-contracts) |

## Components and responsibilities

- `internal/task/service/document_service.go` — unchanged. Storage, revisions,
  coalescing, attachments.
- `internal/task/service/handoff_service.go` — unchanged. Relationship-based
  access decisions for the MCP path.
- `internal/mcp/server/handoff_handlers.go` — tool declarations; `task_id`
  becomes optional and defaults to the calling task.
- `internal/mcp/server/server.go` — the `task-documents` profile entry is
  enabled for kanban as well as office.
- `internal/backendapp/` — new task-scoped mount of the existing document
  handler, with a per-task authorization middleware.
- `apps/web/lib/api/domains/office-extended-api.ts` — the six document functions
  target the task-scoped base path.
- `apps/web/components/task/dockview-shared.tsx` — the plan panel renders the
  existing `TaskDocuments` component under `TaskPlanPanel`.

## Data and contracts

No schema change. No new document type is introduced at the storage layer;
`spike` is an ordinary `type` value, and the frontend adds it to its badge
table alongside `PLAN`, `SPEC`, `NOTES`, `REVIEW`, `ATTACHMENT`.

New HTTP routes, same handler methods and response shapes as the Office ones:

```
GET    /api/v1/tasks/:id/documents
GET    /api/v1/tasks/:id/documents/:key
PUT    /api/v1/tasks/:id/documents/:key
DELETE /api/v1/tasks/:id/documents/:key
GET    /api/v1/tasks/:id/documents/:key/revisions
POST   /api/v1/tasks/:id/documents/:key/revisions/:revId/restore
POST   /api/v1/tasks/:id/documents/:key/upload
GET    /api/v1/tasks/:id/documents/:key/download
```

MCP tool contract change: `task_id` on `list_task_documents_kandev`,
`get_task_document_kandev`, and `write_task_document_kandev` is optional and
defaults to the calling session's task. Office callers that pass an explicit id
are unaffected.

Multiple documents of one type use distinct keys (`spike`, `spike-2`, ...);
`UNIQUE (task_id, key)` already allows this, so only the tool descriptions
change. Plans are not duplicated into documents: `update_task_plan_kandev`
gains an optional `new_revision` boolean that maps to the existing
`UpdatePlanRequest.ForceNewRevision`, so a superseding plan always appends a
revision instead of coalescing into the previous one. Earlier plans are read
through `list_task_plan_revisions_kandev` and `get_task_plan_revision_kandev`.

Conditional Plan Approval routing uses an agent-less human gate step between
Architect and Implement. On first entry, the Architect signals completion via
`step_complete_kandev` to advance to the gate. On an escalation entry from
Implement, the Architect bypasses the gate by default by calling `move_task_kandev`
directly to Implement without signaling turn completion. When human sign-off is
warranted, the Architect signals `step_complete_kandev` with an `APPROVAL REQUESTED:`
summary prefix to park the card at Plan Approval. Because the agent-less gate step
does not carry forward `step_complete` summary text to the next agent, the Architect
records the implementer-facing handoff or advice in the plan's top revision note
whenever parking at the gate. The rule is prompt-enforced without requiring new
engine-level guards.

## Control flow

Agent write: agent → MCP tool → WS `ActionMCPWriteTaskDocument` →
`HandoffService.WriteDocumentForCaller` (relationship check) → `DocumentService`
→ SQLite. Unchanged except for the registration gate and the `task_id` default.

Browser read: Kanban plan panel → `listDocuments`/`getDocument` →
`/api/v1/tasks/:id/documents...` → task authorization middleware →
`dashboard.DocumentHandler` → `DocumentService`.

## Failure and recovery

A document fetch failure renders the component's existing error state; the plan
panel above it continues to render its own content independently. An agent whose
expected document is absent must proceed from the task prompt rather than invent
its content; that is a prompt responsibility, not a runtime one.

## Persistence

Unchanged. Same tables, same revision and coalescing behavior, same attachment
root under `<KANDEV_HOME>/data/attachments/<taskID>/`.

## Security

The Office route group authorizes by resolving each route's resource id to a
workspace in `officeWorkspaceScopeMiddleware`. The `/api/v1` task routes use a
different convention: each handler calls `Service.AuthorizeTaskAccess`. The
office `DocumentHandler` methods contain no such call, so mounting them on
`/api/v1` without a guard would expose every task's documents to any
authenticated user by guessed id.

The new mount therefore installs a group middleware that resolves `:id` and
calls `AuthorizeTaskAccess` before dispatch, failing closed on any resolver
error. This mirrors the fail-closed posture of the Office scope guard and is the
single check that makes the widened surface safe.

Path-component containment for `:id` and `:key` is already enforced in
`DocumentService.safePathComponent` and is unchanged.

## Observability

No new metrics. Authorization rejections follow the existing task-access
rejection logging: a debug entry for an expected denial, error level only for
infrastructure failures.

## Related decisions

- [ADR 0015](../../../decisions/0015-explicit-completion-signal-for-auto-advance.md)
