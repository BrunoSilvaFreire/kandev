---
status: draft
system: agents
requirements:
  - REQ-AGENTS-DYNAMIC-PROFILE-SCHEDULING-001
---

# Dynamic Profile Scheduling System Design

## Purpose and boundaries

This design owns schedule-time ranking for Dynamic Profiles: the eligibility and
ranking order, the soft-affinity semantics, the closed selection reason, the
storage of the preference lists, and the conversion of legacy workflow-step
`allowed_tags` into generated Dynamic Profiles.

It builds on adjacent contracts it does not own:

- [Dynamic Agent Routing](../../decisions/2026-08-13-dynamic-agent-profile-routing.md)
  owns the virtual `dynamic` family, explicit candidates, the conductor,
  circuits and probe leases, route generations, continuation, and post-launch
  fallback.
- `internal/agent/usage` owns provider utilization fetching and
  `usage.RemainingPct`.
- `internal/workflow` and `internal/orchestrator` own workflow-step persistence,
  resolution, and the committed transition ledger.

Ranking runs only for a new route decision. A healthy active route stays sticky.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-AGENTS-DYNAMIC-PROFILE-SCHEDULING-001` | [Preference persistence](#preference-persistence), [Eligibility and ranking](#eligibility-and-ranking), [Resolution path](#resolution-path), [Reasons and observability](#reasons-and-observability), [Legacy tag conversion](#legacy-tag-conversion), [Editor](#editor) |

## Preference persistence

`dynamic_agent_profiles` gains two additive columns:

- `preferred_tags TEXT NOT NULL DEFAULT '[]'`
- `avoided_tags TEXT NOT NULL DEFAULT '[]'`

Both use the same JSON-array TEXT handling as `agent_profiles.tags`. Fresh
databases get the columns in the `CREATE TABLE`; existing databases get
idempotent `ADD COLUMN` migrations under the replayable-migration contract
(ADR 0027). `models.DynamicAgentProfile` gains `PreferredTags []string` and
`AvoidedTags []string`; the store scans and writes the JSON internally.

Canonicalization reuses `internal/common/tags` (`models.CanonicalTags`). A tag
appearing in both lists is rejected at the API boundary with a field-addressable
error. Dynamic profile row tags remain distinct from these routing preferences;
concrete profile tags are unchanged.

## Eligibility and ranking

Eligibility is evaluated before preference. A candidate is ineligible when it
is missing, deleted, disabled, non-launchable, dynamic, a rich Office identity,
`AutoFallback`, executor-incompatible, known-zero quota, or behind a
non-probeable open circuit. A usage fetch error makes a candidate `unavailable`,
which stays eligible below known-positive candidates and above known-zero.

Among eligible candidates the order is:

1. known-positive capacity before unknown/unavailable;
2. within one capacity band, preferred-tag match, then neutral, then avoided;
3. weighted-random by remaining percentage within a band (deterministic when no source of randomness is injected);
4. configured candidate position.

A preferred candidate with no usable capacity never blocks a neutral or avoided
fallback. Empty preference lists reduce to quota-first configured order.

Circuit health is the existing hard gate with its half-open probe lease. Ranking
does not add a parallel health score.

## Resolution path

The ranking lives in the Dynamic Profile load/resolve path, reusing the existing
quota comparator and `usage.RemainingPct`. `internal/agent/selection` remains the
single ranking implementation; no second scoring implementation or strategy
registry is added.

- Candidate tags and bounded quota state/remaining data are added to the
  in-memory `dynamic.Candidate` only. Live percentages are never persisted in
  profile configuration.
- Candidate usage is fetched concurrently before the engine selection lock is
  acquired, reusing cache coalescing and the provider-usage single-fetch
  recorder. An error affects one candidate.
- The existing usage provider is injected into `ProfileExecutionResolver`;
  `backendapp` does not create another client or poller.
- A narrow candidate-eligibility callback lets real task/workflow sessions reuse
  `ValidateAgentProfileForExecutor` against their current executor context.
  Utility calls without a task executor retain launchability/profile validation
  and do not import task packages into `runtime`.
- `dynamic.Engine` keeps circuit/probe admission and generation fencing. It
  consumes the ranked candidates, selects the first circuit-eligible entry, and
  persists the closed reason.
- Retry-with-preference bypasses soft reordering but retains eligibility and
  circuit checks.

## Reasons and observability

The closed route-reason vocabulary adds `preferred_tag_match`, `quota_headroom`,
`configured_order`, `preferred_unavailable_fallback`, and
`avoided_only_fallback`. Existing manual and policy reasons are retained.
Reasons persist through `dynamic_route_attempts.reason` and the session route
projection; no second decision-history table is introduced.

Selection logs remain structured and bounded: step/candidate identity class,
reason, candidate count, and quota-state class. They never log credentials,
credential paths, account IDs, or raw provider payloads.

## Legacy tag conversion

A single idempotent conversion function runs at stored-config migration and at
legacy import/sync boundaries. For each step with non-empty `allowed_tags`:

1. derive a deterministic generated Dynamic Profile identity from the step ID;
2. build explicit candidates from the current concrete profiles matching the
   step tags, plus the step's original fallback profile when distinct;
3. order matching candidates by canonical profile ID and append the neutral
   fallback;
4. copy `allowed_tags` into `preferred_tags`, leave `avoided_tags` empty, and use
   the existing default per-candidate error policies;
5. set `migrated_from` to `workflow_allowed_tags:<step-id>`;
6. update the step's `agent_profile_id` last, so crash/replay is idempotent;
7. retain the original `allowed_tags` during the compatibility release.

A conversion that cannot produce at least one valid candidate fails with an
actionable validation error and does not rewrite the step. When dynamic routing
is disabled during the compatibility release, legacy tagged selection remains
available. When enabled, a step bound to a generated Dynamic Profile resolves
through it and bypasses the legacy selector.

Portable configuration carries the Dynamic Profile's preferred/avoided tags,
ordered concrete candidate descriptors, and policies. Legacy imports that
contain only `allowed_tags` run the same converter against local concrete
profiles.

## Editor

The Dynamic Profile editor presents a soft-preference section before the
candidate list. `TagTokenInput` edits the preferred and avoided lists; existing
candidate cards show concrete tags, capacity state, and the derived preference
class. Desktop uses 28px ordinary controls; phone actions are at least 44px with
safe-area padding and no document horizontal overflow. One draft/save state is
shared across viewports, and all copy is localized in the six catalogs.

## Failure and recovery

Telemetry failure is candidate-local. A route is only refused when no eligible
candidate and no probeable circuit remain. Conversion failures surface as
validation errors and leave the step unchanged. Rollback before cleanup restores
the prior binary with the original `allowed_tags` still present.

## Related decisions

- [Dynamic profile scheduling ownership](2026-09-25-dynamic-profile-scheduling.md)
- [Unify provider routing behind dynamic agent profiles](2026-08-13-dynamic-agent-profile-routing.md)
- [Tagged, quota-aware initial workflow profile selection](2026-09-22-tagged-quota-agent-selection.md)
- [Replayable schema migrations](0027-replayable-schema-migrations.md)
