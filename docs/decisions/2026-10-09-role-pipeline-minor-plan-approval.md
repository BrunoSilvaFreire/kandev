# ADR-2026-10-09-role-pipeline-minor-plan-approval: Audited minor plan revisions

**Status:** accepted
**Date:** 2026-10-09
**Area:** workflow

## Context

The Role Pipeline currently asks for human approval after every plan revision. Repeated Implementer escalations have exposed missing design decisions, while even mechanical clarifications consume a human approval round. Existing completion enrollment requires an exact approved plan head, and the terminal gate has also blocked Architect-to-Implement handoffs before implementation criteria can be verified.

## Decision

Require an evidence preflight before Implement. The first plan and every material revision keep human approval. Permit an Architect-authorized, auditable minor-revision receipt only atop a human-approved baseline, for changes that preserve completion criteria and do not change product, API, persistence, security, destructive behavior, dependencies, or material risk. The backend binds that receipt to exact plan revision and write version and re-enrolls unchanged criteria. A prompt assertion is insufficient. Because free-text intent cannot be classified by the server, "minor" is enforced as a shape: the human-approved baseline content must be an exact prefix and the only addition is a trailing non-normative `## Architect clarifications (advisory)` section. Authority is the MCP-bound session running in the workflow step that obtained the baseline human approval; there is no role-name lookup. A stale or unenrolled plan blocks handoff; unverified implementation criteria block terminal completion, not the handoff into Implement.

## Consequences

Minor clarification can proceed without a Needs-you approval while preserving provenance and gate currency. The backend needs an explicit receipt operation and policy tests; prompt wording alone cannot grant authority. Ambiguous classification fails closed to human review.

## Alternatives Considered

- Auto-approve every Architect edit: rejected because product/risk changes could bypass human decisions.
- Keep human approval for every edit: rejected because it creates unnecessary interruption for unchanged criteria and guidance.
- Treat prompt text as proof of approval: rejected because enrollment and handoff need durable, exact-revision authority.
