---
status: draft
system: tasks
requirements:
  - REQ-TASKS-APPROVAL-REQUESTS-001
  - REQ-TASKS-APPROVAL-REQUESTS-002
  - REQ-TASKS-APPROVAL-REQUESTS-003
---

# Approval Requests System Design

## Purpose and boundaries

The tasks system owns approval because the subject (task plan or task document),
the pending plan comments, and the blocking session turn are all task records.
The design reuses the clarification contract (`internal/clarification`) rather
than introducing an approval store: an approval is a clarification bundle with
`approval` metadata and one fixed question.

Adjacent contracts used but not owned here: the task-plan service (plan version,
plan comments), the document service (document version), and the MCP session
transport.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-APPROVAL-REQUESTS-001` | [Control flow](#control-flow) |
| `REQ-TASKS-APPROVAL-REQUESTS-002` | [Data and contracts](#data-and-contracts), [Failure and recovery](#failure-and-recovery) |
| `REQ-TASKS-APPROVAL-REQUESTS-003` | [Control flow](#control-flow) |

## Components and responsibilities

- **`request_approval_kandev`** (MCP session tool): builds the fixed question and
  forwards a payload carrying `approval` through the existing
  `ask_user_question_kandev` backend action. It shares the keep-alive and
  timeout-notification helper.
- **Approval metadata recorder** (`mcp/handlers`): before the bundle is
  persisted, records the subject's current version via the injected filler
  (`Resolver.FillApprovalVersion`).
- **Clarification resolver** (`internal/clarification`): detects an approval
  bundle, runs approval validation, fills the outcome pre-claim, and consumes
  comments after delivery.
- **Approval support** (`internal/task/service`): implements
  `CurrentVersion`, `RenderPlanComments`, and `ConsumePlanComments` over the
  plan and document services.
- **Web approval card**: renders the subject summary, the Open-plan action, the
  live pending-comment count, the edited-since-request indicator, a feedback
  field, and the three decision buttons.

## Data and contracts

- `Request.Approval` (`ApprovalMeta`): `subject`, `document_key`, `title`,
  `version_at_request`.
- `Answer.PlanCommentRefs`: the exact `{id, version}` references a revise
  answer carries.
- `Response.Approval` (`ApprovalOutcome`): `decision`, `feedback`,
  `plan_comments`, `comment_ids`, `subject_edited`, `current_version`.
- Persistence: the approval metadata rides the bundle's message metadata; the
  filled outcome rides the single question's stored response value, so replay
  and the timeout fallback carry identical content. No new table or column.

## Control flow

1. The agent calls `request_approval_kandev`; the backend records the subject
   version, creates the bundle, and blocks the turn with keep-alive.
2. On answer, the resolver resolves identity and authorizes the bundle's task,
   then runs generic validation followed by approval validation: the bundle
   shape, the decision, the refs, and the edit state.
3. The resolver renders the pending plan comments and fills the outcome
   pre-claim, then claims the bundle and delivers the response.
4. After a successful delivery the resolver consumes the referenced comments
   best-effort.
5. On a lost claim, the winner's stored response is reconstructed, including the
   approval outcome.

## Failure and recovery

- Stale or foreign refs fail validation before the claim, so the client can
  refresh and resubmit.
- A comment-consumption failure is logged and leaves the comments pending; the
  delivered answer is unaffected.
- A rejected bundle maps to the `reject` decision.

## Persistence

No schema change. Approval metadata is stored in the existing bundle message
metadata and the outcome in the existing per-question response value.

## Security

The resolver authorizes through the bundle's durable task id, never a
client-supplied id. The subject version is read server-side, so a caller cannot
forge `subject_edited`.

## Observability

Approval resolution reuses the clarification response-phase logs. Comment
consumption failures log a warning with `pending_id` and `task_id`.

## Related decisions

- [ADR 0015](../../../decisions/0015-explicit-completion-signal-for-auto-advance.md)
