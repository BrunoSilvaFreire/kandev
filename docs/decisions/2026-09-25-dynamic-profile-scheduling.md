# ADR-2026-09-25-dynamic-profile-scheduling: Dynamic Profiles Own Schedule-Time Ranking

**Status:** accepted
**Date:** 2026-09-25
**Area:** backend, frontend, workflow

## Context

Two dynamic-selection mechanisms grew side by side.

[Dynamic agent profile routing](2026-08-13-dynamic-agent-profile-routing.md)
owns post-launch provider fallback, circuits, generations, and continuation, but
its [rejected telemetry package](2026-08-13-dynamic-agent-profile-routing.md)
left schedule-time selection as fixed candidate order.

[Tagged, quota-aware workflow agent selection](2026-09-22-tagged-quota-agent-selection.md)
added a second, workflow-entry scheduler: `allowed_tags` on a step, OR matching
against concrete profile tags, quota ranking, and a frozen per-entry choice. It
has no post-launch fallback, so a tagged step and a dynamic profile each solve
half of the same problem with different owners, settings surfaces, and failure
behavior.

Maintaining both means two ranking implementations, two explainability surfaces,
and two places an operator can choose among interchangeable providers.

## Decision

Make Dynamic Profiles the only long-term owner of schedule-time ranking.

- A Dynamic Profile gains soft `preferred_tags` and `avoided_tags` lists, matched
  against each explicit candidate's existing concrete profile tags. A tag in
  both lists is rejected. Candidates stay explicit; no tag-discovered pool and no
  routing DSL is introduced.
- Eligibility is evaluated before preference. Known-zero, non-probeable open
  circuit, executor-incompatible, and otherwise unlaunchable candidates are
  ineligible. A usage fetch failure is `unavailable`, not ineligible, so
  API-key and unsupported providers stay selectable.
- Ranking order is known-positive capacity, then within a band preferred above
  neutral above avoided, then remaining percentage, then configured position. A
  preferred candidate with no usable capacity never blocks a fallback. Empty
  lists preserve quota-first configured order.
- Circuit state stays the single health gate with its existing half-open probe
  lease. No parallel health score is added.
- Selection persists one reason from a closed vocabulary
  (`preferred_tag_match`, `quota_headroom`, `configured_order`,
  `preferred_unavailable_fallback`, `avoided_only_fallback`, plus retained
  manual/policy reasons) through the existing route-attempt and session route
  projection. No second decision-history table is added.
- Ranking runs only for a new route decision. A healthy active route stays
  sticky; explicit same-candidate retry keeps its policy semantics.
- Stored and portable workflow steps with `allowed_tags` are converted
  deterministically and idempotently into generated Dynamic Profiles. The
  original tags remain stored during the compatibility release. The legacy
  workflow-entry selector is removed only after migration and parity gates pass.

Existing `features.dynamicAgentRouting` remains the single backend-authoritative
kill switch. No tag-scheduling flag is added.

## Consequences

- One ranking implementation, one settings surface, and one explainability
  vocabulary replace two half-schedulers.
- Concrete profile tags stay supported and canonical; only the workflow-step
  `allowed_tags` scheduler is retired.
- Additive `preferred_tags`/`avoided_tags` columns and retained `allowed_tags`
  keep rollback non-destructive: the prior binary can still use the legacy path.
- Generated Dynamic Profiles become visible configuration, so route history
  retains the new logical ID. The `migrated_from` marker preserves provenance.
- Cold provider calls add bounded latency; fetches stay concurrent and outside
  the engine lock.
- Unknown telemetry remains a first-class state; treating it as zero would strand
  unsupported providers.

## Alternatives Considered

- **Keep the tagged workflow scheduler.** Rejected: it cannot own post-launch
  fallback and duplicates ranking, settings, and explainability.
- **Merge tags into the dynamic profile row's own `tags` field.** Rejected: it
  conflates display tags with routing preferences and breaks canonical
  round-tripping.
- **Tag-discovered candidate pools.** Rejected: explicit candidates remain
  authoritative and auditable; discovery introduces implicit configuration.
- **A strategy registry or cost/latency strategies.** Rejected: there is one
  ranking to own, not a family.
- **A parallel health score.** Rejected: circuit state already exists and a
  second score would race its probe leases.
