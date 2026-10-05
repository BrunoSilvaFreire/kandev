---
status: deprecated
system: agents
implementation_plans:
  - docs/plans/tagged-quota-agent-selection/plan.md
requirements:
  - REQ-AGENTS-TAGGED-QUOTA-SELECTION-001
---

# Tagged quota-aware workflow agent selection requirements

> **Deprecated (2026-09-25).** Superseded by
> [Dynamic Profile Scheduling](dynamic-profile-scheduling.md)
> (`REQ-AGENTS-DYNAMIC-PROFILE-SCHEDULING-001`), which owns schedule-time
> ranking. The workflow-step `allowed_tags` scheduler described here is retained
> only during the compatibility release and is removed after the migration and
> parity gates pass. Requirement and acceptance-criterion IDs are retained for
> history.

## Overview

A workflow step can name one concrete agent profile. Operators who keep more
than one interchangeable profile (for example two review subscriptions) must
choose one by hand and keep editing the step as subscriptions run out. This
capability lets a step declare a set of allowed tags and lets Kandev pick, at
each step entry, one eligible concrete profile that still has the most
subscription quota, then hold that choice for the entry. It is opt-in: a step
with no allowed tags behaves exactly as before.

## Terms

- **Tag**: a free-form, canonicalized lowercase label on a concrete agent
  profile.
- **Allowed tags**: the tag list on a workflow step; a profile qualifies when it
  contains at least one allowed tag (OR).
- **Remaining quota**: `100` minus the largest provider window utilization
  percent, clamped to `0..100`. Empty or absent windows mean unknown.
- **Static fallback**: the step's configured `agent_profile_id`, or the existing
  workflow-default and task/session fallbacks used when tags are absent.

## Requirements

### REQ-AGENTS-TAGGED-QUOTA-SELECTION-001: Tagged quota-aware workflow agent selection

Kandev shall let a concrete agent profile carry canonical free-form tags, let a
workflow step declare allowed tags, and choose one eligible concrete profile by
remaining subscription quota at each entry of a tag-configured step, freezing
that choice for the entry while leaving fixed-profile and dynamic-routing
behavior unchanged.

#### Acceptance criteria

- **AC-AGENTS-TAGGED-QUOTA-SELECTION-001.1:** When a profile's tags are created
  or updated through any write boundary, the system shall trim each tag, lower
  its case, reject empty values, deduplicate, sort, and cap the list and each
  tag's UTF-8 byte length before persisting a JSON array default.
- **AC-AGENTS-TAGGED-QUOTA-SELECTION-001.2:** When a profile is created, read,
  updated, duplicated, listed, or reloaded, the system shall round-trip the
  canonical tag list, and rows written before this capability shall read as an
  empty list.
- **AC-AGENTS-TAGGED-QUOTA-SELECTION-001.3:** When a tag write is invalid or
  oversized, the system shall reject it at the API boundary, and a dynamic
  profile shall reject tags.
- **AC-AGENTS-TAGGED-QUOTA-SELECTION-001.4:** When a workflow step declares
  allowed tags, the system shall treat a profile as a candidate when it contains
  at least one allowed tag, and shall not count a multi-tag profile twice.
- **AC-AGENTS-TAGGED-QUOTA-SELECTION-001.5:** When a step has no allowed tags,
  the system shall resolve the concrete profile exactly as before, and clearing
  the allowed tags shall restore that behavior without a restart.
- **AC-AGENTS-TAGGED-QUOTA-SELECTION-001.6:** When both a task fixed-step
  override and allowed tags exist, the task override shall win; otherwise a
  non-empty allowed-tags list shall be resolved before the step profile, the
  workflow default, and the existing task/session/workspace fallbacks.
- **AC-AGENTS-TAGGED-QUOTA-SELECTION-001.7:** When allowed tags are configured,
  the system shall reject them on a step with `session_target` or
  `configure_session`, and shall keep `agent_profile_id` valid alongside them as
  the explicit fallback.
- **AC-AGENTS-TAGGED-QUOTA-SELECTION-001.8:** When ranking candidates, the
  system shall order known remaining quota above zero descending, then unknown
  or unavailable, then known zero, breaking ties by canonical profile ID
  ascending.
- **AC-AGENTS-TAGGED-QUOTA-SELECTION-001.9:** When a candidate's provider usage
  fetch fails, the system shall treat only that candidate as unavailable and
  shall still select from the remaining candidates.
- **AC-AGENTS-TAGGED-QUOTA-SELECTION-001.10:** When every matched candidate is
  known-zero, the system shall use the static fallback only if it is not one of
  those candidates, and shall otherwise fail with a typed no-eligible-profile
  error before any transition commits.
- **AC-AGENTS-TAGGED-QUOTA-SELECTION-001.11:** When a tag-configured step is
  entered, the system shall evaluate quota once and persist the chosen profile
  in the bounded workflow-session route under the committed transition identity
  in the same transaction as the step transition, so a manual move, engine/CAS
  transition, genesis creation, or promotion that commits a tagged destination
  always carries a frozen choice; launch, restart, replay, and reuse shall
  consume that recorded choice without re-ranking, and only a later entry with a
  different identity shall re-evaluate.
- **AC-AGENTS-TAGGED-QUOTA-SELECTION-001.15:** When the read-only workflow-move
  preview is computed, the system shall present any candidate as advisory and
  shall not create an entry or reserve a route, while the actual move repeats
  validation and freezes its own result.
- **AC-AGENTS-TAGGED-QUOTA-SELECTION-001.16:** When a tag-configured step holds
  a task whose current entry predates the tags (no frozen route), the system
  shall fall back to the step's configured `agent_profile_id` so launch,
  restart, replay, and reuse keep working, and shall fail with the typed
  no-frozen-profile error only when the step has no configured profile; it shall
  not re-rank quota for that entry.
- **AC-AGENTS-TAGGED-QUOTA-SELECTION-001.12:** When a provider error occurs
  after launch, the system shall leave dynamic routing as the only owner of
  post-launch fallback and shall not re-run workflow profile selection.
- **AC-AGENTS-TAGGED-QUOTA-SELECTION-001.13:** When the settings surfaces open
  the candidate list, the system shall show each candidate's remaining quota
  state as known, unknown, or unavailable without color-only meaning, on desktop
  and phone, and shall not fetch provider usage during settings boot.
- **AC-AGENTS-TAGGED-QUOTA-SELECTION-001.14:** When allowed tags and profile
  tags are exported, imported, or synced, the system shall preserve them as
  plain portable strings, and tag-free workflows shall serialize unchanged.

## Exclusions

- No change to the Office `role` enum, dynamic-routing post-launch behavior, or
  historical session profile IDs.
- No tag join table, tag index, dynamic-profile candidates, cost or latency
  strategy, strategy registry, runtime flag, or boot-time usage fetch.
- No mid-session rebalancing and no second fallback engine for a launched
  session.
