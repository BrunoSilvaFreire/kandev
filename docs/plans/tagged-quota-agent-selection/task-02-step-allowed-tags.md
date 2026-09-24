---
id: task-02
title: Carry allowed_tags through workflow paths
status: done
wave: 1
depends_on: []
plan: docs/plans/tagged-quota-agent-selection/plan.md
requirements:
  - REQ-AGENTS-TAGGED-QUOTA-SELECTION-001
acceptance_criteria:
  - AC-AGENTS-TAGGED-QUOTA-SELECTION-001.5
  - AC-AGENTS-TAGGED-QUOTA-SELECTION-001.7
  - AC-AGENTS-TAGGED-QUOTA-SELECTION-001.14
system_design:
  - docs/specs/agents/system-design/tagged-quota-agent-selection.md
---

# Task 02: Carry allowed_tags through workflow paths

## Outcome

A workflow step stores a canonical `allowed_tags` list that survives create,
update, clear, duplicate, export, import, sync, MCP config, and the desktop and
phone selectors, while tag-free workflows resolve unchanged.

## Scope

In scope: the `workflow_steps.allowed_tags` column, `WorkflowStep.AllowedTags`,
step CRUD/duplicate/export/import/sync, MCP workflow config handlers, the HTTP
type, the step agent-profile selector UI, and its dirty-state handling.

Excluded: the selection algorithm and usage telemetry.

## Requirements and design

- `REQ-AGENTS-TAGGED-QUOTA-SELECTION-001`
- [System design](../../specs/agents/system-design/tagged-quota-agent-selection.md)

## Acceptance

- Create/update omission, null/clear, persistence, duplication, YAML/JSON
  export/import, sync apply, and MCP config preserve canonical tags.
- `allowed_tags` is rejected alongside `session_target` or `configure_session`,
  while a fallback `agent_profile_id` remains valid.
- A tag-free workflow serializes and resolves unchanged.
- Desktop Popover and phone picker configure the same value.

## Verification

```bash
(cd apps/backend && go test ./internal/workflow/...)
(cd apps/backend && go test ./internal/mcp/handlers -run 'Workflow|Step')
(cd apps/web && pnpm exec vitest run components/settings/workflow-step-agent-profile-selector.test.tsx components/settings/workflow-dirty-state.test.ts lib/types)
```

Stop if import/export requires changing portable `agent_profile` descriptor
semantics; `allowed_tags` must remain plain portable strings.

## Likely files and risks

`internal/workflow/{models,repository,controller,handlers,service}/`,
`internal/mcp/handlers/config_workflow_handlers.go`,
`apps/web/lib/types/http.ts`, the step selector and session-target surfaces, and
locale catalogs.

Risk: several step-write surfaces share a validation path; the
`session_target` rejection must land in the shared validator, not one handler.

## Results

Backend done: `workflow_steps.allowed_tags` column + additive migration,
`WorkflowStep.AllowedTags` / `StepDefinition.AllowedTags` / `StepPortable`
`allowed_tags`, repository create/update/select/scan and template seed JSON
conversion, `ValidateAllowedTags` (canonicalize + reject `session_target` /
`configure_session`) wired through `ValidateWorkflowStep`, controller create
and update requests, MCP step create/update payloads, stepevents payload, and
export/import/sync (including the sync equality check). Tests added in
`workflow/models/allowed_tags_test.go` and
`workflow/repository/allowed_tags_test.go`; all `go test ./internal/workflow/...`
and `./internal/mcp/...` suites pass.

Frontend done: `WorkflowStep`/`StepDefinition` `allowed_tags`, the step
equality/save payload, `WorkflowStepAllowedTags` (token editor plus the
session-target/configure_session conflict copy) rendered in the step panel, the
workflow step-action load/save transforms, and `workflows` copy in all six
locales. Verified by
`tests/workflow/tagged-quota-agent-selection.spec.ts` and
`tests/workflow/mobile-tagged-quota-agent-selection.spec.ts`.
