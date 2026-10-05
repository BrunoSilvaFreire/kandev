---
status: superseded
system: agents
requirements:
  - REQ-AGENTS-TAGGED-QUOTA-SELECTION-001
---

# Tagged quota-aware workflow agent selection system design

> **Superseded (2026-09-25).** Schedule-time ranking moved to
> [Dynamic Profile Scheduling](dynamic-profile-scheduling.md). This design keeps
> describing the temporary legacy bridge until it is removed after the migration
> and parity gates pass.

## Purpose and boundaries

This design covers the `agents`-owned profile tag data and the selection service
that chooses one concrete execution profile when a workflow step declares
`allowed_tags`. It sits on top of existing owned seams: workflow-step
persistence and resolution (`internal/workflow`), workflow session lifecycle
(`internal/orchestrator`), provider usage telemetry
(`internal/agent/usage`), and settings composition (`internal/backendapp`).

It does not own post-launch fallback. [Dynamic agent profile
routing](../../decisions/2026-08-13-dynamic-agent-profile-routing.md) remains the
only owner of retry, circuits, and generations after a session launches.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-AGENTS-TAGGED-QUOTA-SELECTION-001` | [Profile tags](#profile-tags), [Step allowed tags](#step-allowed-tags), [Selection service](#selection-service), [Resolution precedence](#resolution-precedence), [Quota scoring](#quota-scoring), [Entry freezing](#entry-freezing), [Settings surfaces](#settings-surfaces), [Observability](#observability) |

## Profile tags

`agent_profiles` gains a `tags TEXT NOT NULL DEFAULT '[]'` column, mirroring the
existing JSON-array TEXT pattern used by `skill_ids` and `desired_skills` in
`internal/agent/settings/store/sqlite.go`. Fresh databases get the column in the
`CREATE TABLE`; existing databases get an idempotent `ADD COLUMN` in
`migrateOfficeEnrichmentColumns`. The column is listed after the table-rebuild
migration so a legacy rebuild cannot drop it.

`models.AgentProfile` gains `Tags []string` (`db:"-"`); the store
scans/writes the JSON string internally, so no JSON string leaks to callers.
Canonicalization lives in one shared helper: trim, lowercase, reject empty,
deduplicate, sort, cap at 32 tags and 64 UTF-8 bytes per tag, and marshal
`[]string` (never `null`). The helper is applied in the create, update, and
duplicate paths of `internal/agent/settings/controller` and at the HTTP DTO
boundary in `apps/backend/pkg/api/v1/agent.go`, so an invalid value fails with
`400` before storage. Dynamic profiles reject a non-empty tag list.

The web `AgentProfile` type gains `tags: string[]`; the normalizer defaults a
missing value to `[]`; the profile editor renders a token input built from the
existing Input/Button/Badge primitives on desktop and phone.

## Step allowed tags

`workflow_steps` gains an `allowed_tags TEXT NOT NULL DEFAULT '[]'` column with
the same JSON-array TEXT handling, plus `models.WorkflowStep.AllowedTags
[]string`. Step create/update, duplicate, export, import, sync-apply, the MCP
workflow config handlers, and the boot payload all carry the field. Validation
rejects `allowed_tags` on a step that also sets `session_target` or
`configure_session`, because those surfaces configure a concrete session rather
than a selectable profile. `allowed_tags` remains a plain portable string list,
so the portable `agent_profile` descriptor is unchanged.

## Selection service

A new package `internal/agent/selection` owns matching and scoring behind one
narrow interface:

```go
type Candidate struct { ProfileID string; Tags []string }
type Decision  struct { ProfileID string; Reason string; QuotaState string }
type Strategy  interface { Select(ctx context.Context, candidates []Ranked) (Decision, error) }
```

Only `QuotaStrategy` ships. Tag filtering stays in the selector service: it
builds candidates from eligible concrete profiles, filters by OR tag match,
scores each candidate, ranks them, and asks the strategy for the decision. No
persisted strategy enum, registry, or factory is introduced.

Eligibility reuses the same global, enabled, undeleted, launchable concrete
profiles that may be pinned to a Kanban workflow step, excluding dynamic
profiles, rich Office identity rows, and profiles that fail existing
execution-profile validation. Capability and executor-compatibility rules are
not duplicated inside the strategy.

## Resolution precedence

For a step without `session_target`, resolution becomes:

1. task fixed-step override;
2. tag/quota selection when `allowed_tags` is non-empty;
3. step `agent_profile_id`;
4. workflow default;
5. existing task/current-session/workspace fallbacks.

Resolution funnels through the existing `resolveStepAgentProfileForTask*`
family in `internal/orchestrator`. Every caller — preview, launch, restart, and
recovery — must use the error-aware selector so preview and launch cannot
disagree. A tag-selected step is not a valid direct `session_target.kind=step`
source in v1.

## Quota scoring

`internal/agent/usage` already parses provider windows and owns success/failure
caches, credential-key sharing, and 10-second client timeouts. This design adds
one score helper there:

```text
remaining_pct = clamp(100 - max(window.utilization_pct), 0, 100)
```

A nil or empty window set means unknown. Ranking is known-positive descending,
then unknown or unavailable, then known-zero, with canonical profile ID
ascending as the tie-break. A provider fetch error degrades only that candidate
to unavailable.

Fallback:

- no tag match uses the existing static fallback, if any;
- all matched candidates known-zero uses the static fallback only when it is not
  one of those exhausted candidates;
- no safe static fallback returns a typed no-eligible-profile error before the
  transition commits;
- unknown candidates with no known-positive candidate choose the deterministic
  first unknown.

## Shared usage composition

`internal/backendapp` builds one `usageProviderAdapter` regardless of Office
enablement and injects the same instance into Office (when present), the
workflow selector, and a new authenticated bounded batch-utilization endpoint
for settings. Candidate fetches run concurrently under the request context and
retain credential-path cache sharing. The Claude/Codex registration switch is
not refactored.

The settings API is `POST /api/v1/agent-profiles/utilization`, registered before
`/agent-profiles/:id` and owned by `internal/agent/settings/handlers.Handlers`.
The authenticated request is `{"profile_ids":[...]}`; IDs are trimmed and
deduplicated, and empty input or more than 50 unique IDs is rejected. The
response returns one deterministic profile-ID-sorted item per requested ID with
`state: known | unknown | unavailable` and optional raw utilization and
remaining percentage. `nil,nil` from the adapter is unknown; a fetch error is
unavailable; windows produce known through `usage.RemainingPct`. Fetches are
concurrent and bounded under the request context and retain credential-path
cache sharing. The surface is loaded when the candidate list opens, never during
settings boot.

## Entry freezing

The selector chooses only the initial concrete profile for an entry. The freeze
record is the existing bounded `task.metadata[workflow_session_route]` value: a
normal tag-selected route carries `TargetKind="profile"`, the destination step,
the selected `AgentProfileID`, the start policy, an optional source session, and
the exact `EntryIdentity` `entry:%020d` derived from the committed
`task_step_transitions.id`. `OperationID` remains the deterministic
`workflowSessionRouteID(...)`.

The transition transaction owns identity creation. Actual entry preflight
resolves the task override or tag selection once and attaches the smallest
context-carried pending-route template, following the existing
`internal/workflow/stepentry` pattern. The shared
`internal/task/repository/sqlite.recordStepTransition` chokepoint fills
`EntryIdentity`/`OperationID` after the ledger insert returns its ID and updates
the route metadata in the same transaction. This covers manual moves,
engine/CAS transitions, genesis creation, and WIP/feeder promotion without a new
table; a transition that loses a CAS race or is diverted to another step does
not persist the pending route.

One entry-aware orchestrator resolver applies the task fixed-step override,
then, for tagged steps, reuses an exact route match by destination step, entry
identity, target kind, and start policy before any quota call. Only an actual
new-entry preflight scores candidates. `prepareWorkflowStepSession`,
`preflightWorkflowStepCredentials`, `autoStartTaskForLoadedStep`, queue/replay/
session-ensure paths, and `prepareExplicitWorkflowStartRoute` consume the
recorded profile; existing fixed-profile callers stay unchanged.

When a tagged step has no exact route match for the task's current entry (the
entry predates the tags, or the route belongs to another entry or start
policy), the resolver returns the step's own trimmed `agent_profile_id`
without scoring (AC-001.16). It does not consult the workflow default or later
fallbacks, and it does not persist a route. It returns
`entryroute.ErrNoFrozenProfile` only when that step profile is empty. The
resolver order is: task fixed-step override, exact frozen or pending route,
step `agent_profile_id`, typed error. This fallback applies only to the
entry-aware launch resolver. New-entry selection and the read-only preview keep
the selector's no-safe-fallback semantics.

`PreviewWorkflowMove` is read-only and advisory: it may run the selector to show
a candidate, but it creates no entry and reserves no route. Quota is not
re-ranked after the route is persisted, on restart/replay/reuse, while a session
runs, or after a provider error. A later entry re-evaluates.

## Failure and recovery

Telemetry failure is candidate-local. Settings boot and workflow entry never
fail solely because telemetry is unavailable; an entry fails only when no
eligible profile remains and no safe static fallback exists.

## Persistence and migration

Both columns are additive `TEXT NOT NULL DEFAULT '[]'`; code rollback leaves
inert columns. Clearing `allowed_tags` restores legacy fixed-profile behavior
and is the operational kill switch. Migrations follow
[ADR 0027](../../decisions/0027-replayable-schema-migrations.md), with
fresh-DB plus same-DB replay coverage for SQLite.

## Observability

One structured selection log records step ID, chosen profile ID, strategy,
reason, candidate count, and quota-state class. It never logs credentials,
credential paths, account IDs, raw provider payloads, or high-cardinality
metrics. No new metric is added in v1.

## Related ADRs

- [Tagged, quota-aware initial workflow profile
  selection](../../decisions/2026-09-22-tagged-quota-agent-selection.md)
- [Dynamic agent profile
  routing](../../decisions/2026-08-13-dynamic-agent-profile-routing.md)
- [Replayable schema migrations](../../decisions/0027-replayable-schema-migrations.md)
