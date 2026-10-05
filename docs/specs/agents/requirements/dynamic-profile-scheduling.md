---
status: draft
system: agents
created: 2026-09-25
updated: 2026-09-25
owners:
  - cfl
---
# Dynamic Profile Scheduling Requirements

## Overview

Dynamic Profiles are the single long-term abstraction for choosing among
interchangeable agent providers. A dynamic profile lists its concrete candidates
explicitly and already owns post-launch retry, circuits, and generations. This
capability extends the *initial* selection to be quota- and tag-aware without
adding a second scheduler.

An operator who keeps several interchangeable profiles (for example two review
subscriptions) can mark soft tag preferences on the dynamic profile. Kandev
ranks the profile's explicit candidates using existing live quota data and
circuit health, picks the best eligible candidate, and records an explainable
selection reason. Tag preferences are soft: capacity and health can still choose
a fallback.

This replaces the earlier workflow-step `allowed_tags` scheduler as the
long-term owner of schedule-time ranking. Existing tagged steps receive a
bounded conversion into generated Dynamic Profiles.

## Terminology

- **Concrete candidate**: a launchable concrete agent profile explicitly listed
  on a dynamic profile.
- **Preferred tags / avoided tags**: soft tag lists on the dynamic profile,
  matched against each candidate's existing concrete profile tags. They never
  discover candidates.
- **Eligible**: passes every hard gate (see the system design). Preference is a
  ranking input, not an eligibility gate.
- **Capacity band**: known-positive quota, unknown/unavailable, or known-zero.
- **Selection reason**: one value from a closed vocabulary persisted with the
  route decision.

## Requirements

### REQ-AGENTS-DYNAMIC-PROFILE-SCHEDULING-001: Dynamic Profile scheduling preferences

**Intent:** Let a Dynamic Profile's explicit candidates be ranked at initial
selection by live capacity and soft tag affinity, with a closed explainable
reason, while concrete profiles keep canonical free-form tags.

**User story:** As an operator, I want to mark preferred and avoided tags on a
dynamic profile so that Kandev prefers my intended provider while capacity and
health can still choose a fallback.

#### Acceptance criteria

- **AC-AGENTS-DYNAMIC-PROFILE-SCHEDULING-001.1:** When a dynamic profile is
  created, read, updated, listed, or reloaded, the system shall round-trip a
  canonical `preferred_tags` list and a canonical `avoided_tags` list, reject a
  tag present in both lists at the API boundary with a field-addressable error,
  and read rows written before this capability as empty lists.
- **AC-AGENTS-DYNAMIC-PROFILE-SCHEDULING-001.2:** When ranking a dynamic
  profile's candidates, the system shall treat missing, deleted, disabled,
  non-launchable, dynamic, rich-Office, and `AutoFallback` candidates,
  executor-incompatible candidates, known-zero-quota candidates, and candidates
  behind a non-probeable open circuit as ineligible, and shall treat a usage
  fetch failure as unavailable rather than a selection failure.
- **AC-AGENTS-DYNAMIC-PROFILE-SCHEDULING-001.3:** When more than one candidate
  is eligible, the system shall rank known-positive capacity above
  unknown/unavailable; within one capacity band, preferred-tag matches above
  neutral candidates above avoided-tag matches; then known remaining percentage
  descending; then configured candidate position.
- **AC-AGENTS-DYNAMIC-PROFILE-SCHEDULING-001.4:** When a preferred-tag candidate
  has no usable capacity, the system shall not let it block a neutral or avoided
  eligible fallback, and a profile with no preference lists shall preserve
  quota-first, configured-order behavior.
- **AC-AGENTS-DYNAMIC-PROFILE-SCHEDULING-001.5:** When a candidate is selected,
  the system shall persist exactly one reason from the closed vocabulary
  (`preferred_tag_match`, `quota_headroom`, `configured_order`,
  `preferred_unavailable_fallback`, `avoided_only_fallback`, plus the retained
  manual and policy reasons) through the existing route-attempt and session route
  projection, and shall not add a second decision-history table.
- **AC-AGENTS-DYNAMIC-PROFILE-SCHEDULING-001.6:** When a route is already active
  and healthy, the system shall not re-rank or rebalance it mid-turn, and an
  explicit same-candidate retry shall keep its policy semantics without soft
  reordering.
- **AC-AGENTS-DYNAMIC-PROFILE-SCHEDULING-001.7:** When a workflow step has
  non-empty `allowed_tags`, the system shall convert it deterministically and
  idempotently into a generated Dynamic Profile whose explicit candidates are the
  matching concrete profiles plus the step's distinct fallback, with
  `allowed_tags` copied into `preferred_tags`, the original tags retained for the
  compatibility release, and the step bound to the generated profile.
- **AC-AGENTS-DYNAMIC-PROFILE-SCHEDULING-001.8:** When a conversion cannot build
  at least one valid candidate, the system shall fail with an actionable
  validation error and shall not bind an arbitrary profile or rewrite the step.
- **AC-AGENTS-DYNAMIC-PROFILE-SCHEDULING-001.9:** When dynamic routing is
  disabled, the existing legacy tagged selection shall remain available for the
  compatibility release; when it is enabled and a step points at a generated
  Dynamic Profile, the step shall resolve through the Dynamic Profile and bypass
  the legacy selector.
- **AC-AGENTS-DYNAMIC-PROFILE-SCHEDULING-001.10:** When the Dynamic Profile
  editor is shown on desktop or phone, the system shall present preferred/avoided
  tag editing and per-candidate capacity plus derived preference state without
  color-only meaning, and shall share one draft/save state across viewports.

## Out of scope

- Tag join tables or indexes, and tag-discovered automatic candidate pools.
- Cost or latency strategies and any strategy registry or factory.
- A parallel health score, usage poller, or decision-history table.
- Mid-session rebalancing or movement back to a recovered preferred provider.
- Changes to Office role identity, provider-error effect safety, continuation, or
  historical session attribution.

## Related specifications

- [Dynamic Agent Routing](dynamic-agent-routing.md) owns the Dynamic Profile
  family, the conductor, circuits, and post-launch fallback.
- [Tagged quota-aware workflow agent selection](tagged-quota-agent-selection.md)
  is deprecated by this capability; its workflow scheduler is removed after the
  migration and parity gates pass.
