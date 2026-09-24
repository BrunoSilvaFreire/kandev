---
id: task-01
title: Persist and edit canonical profile tags
status: done
wave: 1
depends_on: []
plan: docs/plans/tagged-quota-agent-selection/plan.md
requirements:
  - REQ-AGENTS-TAGGED-QUOTA-SELECTION-001
acceptance_criteria:
  - AC-AGENTS-TAGGED-QUOTA-SELECTION-001.1
  - AC-AGENTS-TAGGED-QUOTA-SELECTION-001.2
  - AC-AGENTS-TAGGED-QUOTA-SELECTION-001.3
system_design:
  - docs/specs/agents/system-design/tagged-quota-agent-selection.md
---

# Task 01: Persist and edit canonical profile tags

## Outcome

A concrete agent profile round-trips a canonical tag list through storage, API,
and the settings editor, with old rows reading as empty and invalid input
rejected at the boundary.

## Scope

In scope: the `agent_profiles.tags` column and additive migration, the
`models.AgentProfile.Tags` field and shared canonicalizer, the settings
controller create/update/duplicate/read paths, the HTTP DTO and API type, the
web profile type/normalizer, and the desktop/phone profile editor token input.

Excluded: candidate selection, quota telemetry, and workflow step fields.

## Requirements and design

- `REQ-AGENTS-TAGGED-QUOTA-SELECTION-001`
- [System design](../../specs/agents/system-design/tagged-quota-agent-selection.md)

## Acceptance

- Fresh schema and same-DB migration both yield `tags` defaulting to `[]`, and a
  pre-existing row reads `[]`.
- Create, update, duplicate, list, and reload round-trip canonical tags
  (trimmed, lowercased, deduplicated, sorted, bounded).
- Invalid or oversized tags fail with a boundary error, and a dynamic profile
  rejects tags.
- The desktop and phone editor persist tags.

## Verification

```bash
(cd apps/backend && go test ./internal/agent/settings/...)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test -race ./internal/persistence/storeconformance -count=1)
(cd apps/web && pnpm exec vitest run lib/state/slices/settings/types.test.ts components/settings/agent-profile-page-state.test.ts components/settings/profile-edit)
```

Stop if `KANDEV_TEST_POSTGRES_DSN` is unavailable; record the gap and do not
claim cross-dialect migration verification.

## Likely files and risks

`internal/agent/settings/{models,store,controller,dto}/`,
`pkg/api/v1/agent.go`, `apps/web/lib/types/agent-profile.ts`, the normalizer,
the settings slice types, `components/settings/profile-edit/`, and locale
catalogs.

Risk: the store uses manual scans; the JSON conversion must stay inside the
store so no JSON string reaches callers.

## Results

Backend done: `agent_profiles.tags` column + additive migration, canonicalizer
in `internal/common/tags` (re-exported by `agent/settings/models`), store
insert/update/select/scan JSON conversion, controller create/update/duplicate
canonicalization with `ErrInvalidProfileTags`, dynamic-profile rejection, DTO
and contract field, and MCP reuse via the shared DTO. Tests added in
`models/tags_test.go`, `store/sqlite_tags_test.go`, and
`controller/profile_tags_test.go`; `go test ./internal/agent/settings/...`
passes.

Frontend done: `AgentProfile.tags`/`AgentProfilePayload.tags`, normalizer pick
+ payload inverse, dirty state, reconciliation editable fields, the shared
`TagTokenInput`, the `ProfileTagsSection` editor on desktop and phone, the save
action/payload carrying tags, and copy in all six locales.

Verification: `agent-profile-normalize.test.ts`, `agent-profile-dirty.test.ts`,
`workflow-dirty-state.test.ts`, `lib/agent-profile-tags.test.ts` pass;
`tests/settings/agent-profile-layout.spec.ts` tag persistence and
`tests/settings/mobile-agent-profile-layout.spec.ts` pass on host Chromium and
`mobile-chrome`.
