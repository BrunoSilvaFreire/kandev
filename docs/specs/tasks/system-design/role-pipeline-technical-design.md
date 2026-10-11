---
status: draft
system: tasks
requirements:
  - REQ-TASKS-ROLE-DESIGN-001
created: 2026-10-06
owners:
  - kandev
---

# Role Pipeline Technical Design

## Scope and baseline

This document is the Architect's technical reference for task `037bb509-5d47-4000-87a4-2ee868902624` and an example of the artifact required by `REQ-TASKS-ROLE-DESIGN-001`. The task plan owns delivery order and completion increments; this document owns implementation direction. The current branch is `main` at `dbb275b7e91cc38cddadd3431a81f6da40aa9136`. The working tree contains uncommitted Implementer changes. Recheck HEAD, the diff, the live workflow, and the deployed tool schema before implementation; preserve every uncommitted change.

The existing completion gate in `apps/backend/internal/task/repository/sqlite/completion_gates.go` is the terminal authority. The existing plan HEAD and revision write path in `apps/backend/internal/task/service/plan_service.go` can coalesce revisions. The approval resolver captures a HEAD `write_version`, but the current enrollment path does not prove that a particular revision was approved. The Implementer handoff `handoff-implement-to-architect-2` records the changed files and prior test evidence. The paired increment contract is [Plan increment completion](plan-increment-completion.md).

## Decisions and direction

1. **Separate artifacts.** The Architect writes a concise task plan and this detailed Markdown design. Publish the exact design content with `write_task_document_kandev(document_key="technical-design", type="custom")`. The plan links this repository path and names the task-document key. The handoff tells Implement and Review to read the design. The task document is a portable copy, not a competing source of technical decisions: update and check it against the file before handoff.
2. **Approval identity.** Bind a winning human `approve` answer to task ID, plan revision ID, and the HEAD `write_version` captured at request time. Persist the receipt with the durable clarification claim. Reject edited, missing-version, stale, rejected, or revised answers as enrollment authority. A revision ID alone is insufficient because revision coalescing can change its content.
3. **Typed increment registry.** Use `task_completion_criteria` for stable increment IDs, descriptions, and repository-issued `CriterionRevision`. Enrollment validates the approval receipt and current plan identity inside the same repository transaction that updates criteria. Preserve evidence for unchanged criteria; a changed criterion gets a new revision; weakening an unmet criterion requires the existing human confirmation.
4. **One terminal gate.** Keep `guardTaskCompletionTransitionTx` as the terminal guard. Scoped Role Pipeline tasks with an unenrolled or stale plan binding fail closed, including the approval-to-enrollment gap. A partial Review PASS verifies only the reviewed increment and returns to Implement through a nonterminal transition. A fully verified plan may use `pass` to Done.
5. **Advisory question detection.** Project `possible_question` only after a session settles to WAITING_FOR_INPUT without a structured pending action or in-turn move. It is a clearable hint, never a synthetic clarification or completion blocker.
6. **Triage scope.** Deliver the possible-question lane, ordering, deduplication, and one in-app no-progress nudge as an independently verifiable increment. Keep the hint separate from clarification-backed Needs-you items and open its conversation rather than an answer form. Include stale Review and interrupted tasks in the same ordered triage surface. Preserve existing `session.turn_finished` event and subscription behavior. The needs-you/parked/done notification split and parked delivery changes belong to the separate parked-notification-deferral contract after its dependencies and open findings are resolved; they are not acceptance conditions for this increment.
7. **Approval churn.** Batch comments into one revision. A new plan requires human approval. After that baseline, a minor clarification may use a dedicated Architect-authorized self-approval operation, never a free-text or prompt-only claim. The operation records a task-scoped audit receipt bound to the exact revision ID and write version, the previous approved head, the actor/session, a concise diff summary and attestation, and rejects changed criterion IDs/descriptions/evidence subjects. It cannot silently promote a plan with changed product definitions, API, persistence, security, destructive behavior, dependencies, or risk; the Architect must classify those as major and request a human decision. If the implementation cannot enforce the declared boundary sufficiently, fail closed to human approval. Free-text Markdown cannot be classified, so the server enforces shape rather than meaning: (a) **minor shape**: the new content equals the human-approved baseline content byte-for-byte, followed only by one trailing section headed exactly `## Architect clarifications (advisory)`; earlier self-approved clarifications must remain a verbatim prefix. Clarifications are non-normative guidance; they cannot amend increments, gates, or stop conditions, and Implement/Review treat a conflict with normative text as an escalation. Any other diff requires human approval. (b) **actor**: the calling session is taken from the MCP transport binding (the same binding `request_approval_kandev` uses), never a parameter. It must belong to the task, be the task's active session, and run in the same workflow step as the session that requested the winning human baseline approval. No step-name or role lookup is used. Rejections return a stable code (`minor_approval_not_baseline_prefix`, `minor_approval_criteria_changed`, `minor_approval_actor_not_plan_owner`, `minor_approval_no_human_baseline`, `minor_approval_stale_head`). See [ADR-2026-10-09-role-pipeline-minor-plan-approval](../../../decisions/2026-10-09-role-pipeline-minor-plan-approval.md).

8. **Preflight and handoff.** Before first Implement entry, the Architect checks each increment against the owning requirement/design, real code paths, data owner, migration and transaction boundary, state/recovery behavior, and runnable verification. Material uncertainty goes to Spike or human approval; minor missing details are resolved and recorded before handoff. The handoff names exact revision, design document, resume point, evidence, and open assumptions. Implementation criteria gate terminal Done, not Architect-to-Implement movement; plan enrollment freshness still gates that movement.

## Contract sketches

The following shapes show the intended boundaries; names and migration details must be reconciled with the current code before implementation.

```go
type PlanApprovalReceipt struct {
    TaskID         string
    PlanRevisionID string
    WriteVersion   string
    RequestID      string
    AnswerID       string
}

type EnrollTaskPlanIncrementsRequest struct {
    PlanRevisionID   string
    ExpectedRevision int64
    Increments       []TaskPlanIncrementDeclaration
}
```

Repository enrollment follows one transaction, including the optimistic completion-set revision check:

```text
BEGIN
  read current plan HEAD, revision, and winning approval receipt for this task
  require equal revision IDs and write versions, and either an unedited human approve answer or an authorized minor-revision receipt on a previously human-approved baseline
  require current completion-set revision == expected revision
  reconcile typed increments; retain unchanged criterion revisions and evidence
  persist plan binding, criteria, and audit history
COMMIT
```

The request and answer identities must come from the clarification repository, not an agent argument. Replayed or racing answers cannot create a second receipt. If receipt insertion fails, the winning claim must also fail or remain recoverable without an enrollable receipt. The exact migration and claim transaction are the first Package 0 design gate.

## Workflow artifact contract

The checked-in Architect prompt in `docs/examples/role-pipeline.workflow.yml` and the guarded live-workflow updater in `scripts/update-live-role-pipeline.py` must give the same instruction: write the technical design file, publish its exact content as the `technical-design` task document, and add both references to the task plan before requesting approval. On escalation, revise the design if a technical decision changed; keep the prior document revision readable. Implement and Review prompts must read the linked document and report a missing or divergent design rather than guessing. The workflow update must detect user-edited prompts and refuse an unreviewed overwrite.

Use the existing task-document API and authorization. No new document type, UI surface, or approval subject is needed. A document write is not itself evidence that the plan or design was approved; approval remains attached to the plan's current revision and write version.

## Coding guidance

- Extend task repository, service, and MCP boundaries already used by completion criteria and clarification. Keep approval proof and gate checks repository-owned; prompts are instructions, not security controls.
- Put schema migrations and read/write transactions in the SQLite repository. Require the same task scope at every lookup. Use optimistic revisions and durable IDs; never derive evidence currency from timestamps or freeform Markdown.
- Keep the `plan_increment` subject's revision equal to the repository-issued criterion revision. Reuse the existing human confirmation and override paths with audit history.
- Keep the possible-question detector conservative and its state clearable by turn/session identity. Structured clarifications outrank hints. Preserve parked-notification deferral.
- Keep the triage nudge in the task UI and deduplicate it by the surfaced hint identity; it must not dispatch or suppress a provider notification. Test the lane on desktop and native mobile without changing the existing notification event.
- Batch plan comments once per approval cycle and preserve revision history. Require a human receipt for the initial or material revision; require an audited, server-validated minor receipt for a qualifying clarification. Re-enroll unchanged criteria against the exact head before handoff.
- Add focused tests for each changed contract and failure path. For UI work, use localized strings, preserve desktop and native mobile capability, and use the guarded Playwright runner. Do not repair unrelated code or commit the existing working tree.

## Verification and stop gates

First verify approval request capture, winning-answer atomicity, enrollment races, stale plan writes, and terminal move blocking in repository and resolver tests. Then run the targeted workflow-example, MCP, status-summary, frontend, and specification checks listed in the task plan. The Implementer handoff contains prior results; rerun affected tests after changing code.

Stop and return to Architect if approval and receipt cannot share a durable claim boundary, if an unenrolled or stale scoped plan can reach Implement or Done, if a live workflow update would overwrite user edits, or if deployed tools cannot satisfy the prompt contract. Do not weaken the completion gate to make a transition pass.
