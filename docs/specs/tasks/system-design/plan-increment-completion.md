---
status: draft
system: tasks
requirements:
  - REQ-TASKS-PLAN-INCREMENT-001
  - REQ-TASKS-PLAN-INCREMENT-002
  - REQ-TASKS-PLAN-INCREMENT-003
  - REQ-TASKS-PLAN-INCREMENT-004
  - REQ-TASKS-PLAN-INCREMENT-005
created: 2026-10-05
owners:
  - kandev
---

# Plan Increment Completion Gate and Possible Question Hint System Design

## Purpose and boundaries

This design establishes typed plan increment verification for Role Pipeline workflows and introduces
a lightweight, non-authoritative `possible_question` projection. It builds upon the existing
`TaskCompletionCriterion` and `guardTaskCompletionTransitionTx` infrastructure.

The task system owns completion criteria storage, evidence evaluation, transition guards, and status
summary projections. Workflows and agent prompts consume these contracts; they do not construct
parallel gates or bypass the repository guard.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-PLAN-INCREMENT-001` | [Increment enrollment and plan reconciliation](#increment-enrollment-and-plan-reconciliation) |
| `REQ-TASKS-PLAN-INCREMENT-002` | [Plan increment evidence subject lifecycle](#plan-increment-evidence-subject-lifecycle) |
| `REQ-TASKS-PLAN-INCREMENT-003` | [Terminal completion gate and move classification](#terminal-completion-gate-and-move-classification) |
| `REQ-TASKS-PLAN-INCREMENT-004` | [Possible question heuristic and projection](#possible-question-heuristic-and-projection) |
| `REQ-TASKS-PLAN-INCREMENT-005` | [Approval receipt and fail-closed binding](#approval-receipt-and-fail-closed-binding) |

## Plan increment evidence subject lifecycle

### Subject identity

A new constant is added to `apps/backend/internal/task/models/completion_gates.go`:
```go
const (
    TaskCompletionEvidencePlanIncrement = "plan_increment"
)
```

For a criterion representing an increment (such as `I1`):
- `Criterion.ID`: Stable increment identifier (e.g. `"I1"`).
- `Criterion.EvidenceSubject`:
  ```json
  {
    "kind": "plan_increment",
    "id": "I1"
  }
  ```
- `Criterion.CriterionRevision`: Integer assigned by the repository when the criterion is inserted or modified (starts at 1).

### Verification and currency check

When an agent or operator submits evidence for an increment:
- `Evidence.Subject`:
  ```json
  {
    "kind": "plan_increment",
    "id": "I1",
    "revision": "1"
  }
  ```
The revision string must match the decimal string of the criterion's current `CriterionRevision`.

In `apps/backend/internal/task/repository/sqlite/completion_gates.go`, `evidenceSubjectCurrentTx` evaluates:
```go
case models.TaskCompletionEvidencePlanIncrement:
    var liveRevision int64
    err := tx.QueryRowContext(ctx, r.db.Rebind(`
        SELECT criterion_revision FROM task_completion_criteria
        WHERE task_id = ? AND criterion_id = ?
    `), taskID, subject.ID).Scan(&liveRevision)
    if errors.Is(err, sql.ErrNoRows) {
        return false, nil
    }
    return err == nil && subject.Revision == strconv.FormatInt(liveRevision, 10), err
```

Because `liveRevision` is scoped to `(task_id, criterion_id)`:
1. Subsequent task updates (e.g., changes to `tasks.updated_at`, workflow moves, or comments) do not touch `criterion_revision` and do not invalidate verified increments.
2. Changes to sibling criteria (such as verifying `I2`) do not alter `I1`'s revision.
3. Modifying `I1`'s definition bumps `liveRevision`, rendering prior evidence stale immediately.

## Increment enrollment and plan reconciliation

### Registry model

The existing `task_completion_criteria` rows act as the typed increment registry. To prevent freeform
Markdown tampering, criteria are registered via an explicit enrollment API tied to an approved plan revision:
```go
type EnrollTaskPlanIncrementsRequest struct {
    WorkspaceID      string                          `json:"workspace_id"`
    PlanRevisionID   string                          `json:"plan_revision_id"`
    ExpectedRevision int64                           `json:"expected_revision"`
    Increments       []TaskPlanIncrementDeclaration  `json:"increments"`
}

type TaskPlanIncrementDeclaration struct {
    ID          string `json:"id"`
    Description string `json:"description"`
}
```

### Plan revision reconciliation

When a plan revision is approved:
1. Unchanged increments preserve their existing `CriterionRevision` and any verified evidence.
2. Modified increments (whose descriptions changed) receive an incremented `CriterionRevision`, clearing `verified_revision` and moving to unverified/stale.
3. Added increments are appended with `CriterionRevision = 1`.
4. Removing or weakening an unmet increment requires an authenticated human confirmation with reason (`models.TaskCompletionHumanConfirmation`), enforcing the existing safety invariant.

## Terminal completion gate and move classification

### Gate enforcement

The existing `guardTaskCompletionTransitionTx` interceptor runs on all state updates transitioning into `COMPLETED`.
If any enrolled criterion has `verified_revision != criterion_revision` or carries stale evidence:
- The transition fails with `repoerrors.ErrTaskCompletionGateBlocked`.
- The human override mechanism (`models.TaskCompletionMoveOverride`) remains available for manual intervention with recorded audit.

### Move error classification

In `apps/backend/internal/mcp/handlers/config_task_handlers.go`, `classifyMoveTaskError` handles
`ErrTaskCompletionGateBlocked` distinctly:
- Emits `ws.ErrorCodeTaskCompletionGateBlocked`.
- Message lists unverified increment IDs, current workflow step, and allowed non-terminal moves (e.g. `request_changes`).
- Directs the caller to resolve unverified increments or request human approval.

## Possible question heuristic and projection

### Detection predicate

At session settlement in `updateTaskSessionStateWithHook`:
When `newState == v1.TaskSessionStateWaitingForInput`:
1. Check that `primary_session_pending_action` is empty.
2. Check that the turn issued no workflow move and no step completion signal.
3. Inspect the turn's final message authored by an agent (`author_type == 'agent'` and `type == 'message'`).
4. If trailing sentences match high-precision decision cues (e.g., "(a)... or (b)...", "Decision for you:", trailing question mark requesting human action), mark `possible_question = true`.

### Projection and precedence

`TaskStatusSummary` gains an additive field:
```go
type TaskStatusSummary struct {
    // ...
    PossibleQuestion bool `json:"possible_question,omitempty"`
}
```

Precedence in UI triage:
1. Real permissions / structured clarifications (`pending_action == "clarification" | "permission"`).
2. Parked background work (`parked_on_background_work == true`).
3. Advisory hints (`possible_question == true`).

Clearing rule:
Any new user message, next turn initiation, step transition, or session completion resets `PossibleQuestion` to `false`.

## Approval receipt and fail-closed binding

### Approval receipt schema

A dedicated table `task_plan_approval_receipts` stores immutable human approval receipts:
```sql
CREATE TABLE IF NOT EXISTS task_plan_approval_receipts (
    id TEXT PRIMARY KEY,
    task_id TEXT NOT NULL,
    plan_revision_id TEXT NOT NULL,
    write_version TEXT NOT NULL,
    decision TEXT NOT NULL,
    subject_edited INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL,
    FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_plan_approval_receipts_lookup
    ON task_plan_approval_receipts(task_id, plan_revision_id, write_version);
```

### Request creation and version capture

When `request_approval_kandev` is called with `subject == "task_plan"`:
1. `FillApprovalVersion` records the current plan's HEAD `write_version` (`meta.VersionAtRequest`).
2. `ApprovalMeta` additionally captures `meta.PlanRevisionID` referencing the current plan revision.
3. Both fields are durably serialized into the clarification message metadata.

### Atomic claim and receipt persistence

In `CompleteActiveClarificationBundle` (`apps/backend/internal/task/repository/sqlite/message_clarification_response.go`),
within the single database write transaction that atomically claims the bundle:
1. If the message metadata indicates `subject == "task_plan"` and `status == "answered"`:
2. The resolver checks the human answer:
   - If `decision == "approve"` AND `subject_edited == false` AND current plan HEAD `write_version == meta.VersionAtRequest`:
     an immutable receipt is inserted into `task_plan_approval_receipts`.
   - If `decision != "approve"` OR `subject_edited == true` OR version has advanced, no receipt is persisted.
3. The receipt insertion shares the transaction with the claim, guaranteeing that an un-claimed or failed response never leaves a dangling receipt.

### Fail-closed enrollment and stale detection

1. **Enrollment Transaction:**
   When criteria are set by an agent (`change.ActorKind == "agent"`):
   - A non-empty `plan_revision_id` is required.
   - The repository verifies in the write transaction that:
     a) A matching receipt exists in `task_plan_approval_receipts` with `decision = "approve"` and `subject_edited = 0`.
     b) The task's current plan HEAD `write_version` matches the receipt's `write_version`.
     c) The `task_plan_revisions` row for `plan_revision_id` also matches `write_version`.
   - On success, `task_completion_sets` records both `plan_revision_id` and `plan_write_version`.

2. **Stale Binding Detection:**
   Any subsequent write to the task plan (`CreateTaskPlan`, `UpdateTaskPlan`, `WritePlanRevision`) generates a new `write_version` on `task_plans`.
   When `readTaskCompletionGateTx` executes:
   - If `task_completion_sets.plan_revision_id != ""` and `task_completion_sets.plan_write_version != task_plans.write_version`, the gate evaluates to `Blocked = true` with blocker `plan_revision_stale: plan was modified after criteria enrollment`.
   - While stale, terminal completion and Architect-to-Implement handoffs fail closed until explicit re-approval and criteria reconciliation occur.

## Verification and test boundaries

- Unit and repository tests in `internal/task/repository/sqlite/completion_gates_test.go`:
  - `plan_increment` validation and verification.
  - Resilience against task updates, moves, and sibling verification.
  - Invalidation when increment definition changes.
  - Human confirmation requirement for removing unmet criteria.
- Service tests in `internal/task/service/`:
  - Enrollment binding to plan revision.
  - Non-terminal move allowed while terminal move blocked.
- MCP tests in `internal/mcp/handlers/`:
  - Actionable error payload on blocked terminal move.
- Status summary tests in `internal/task/statussummary/`:
  - `possible_question` projection and clearance.
