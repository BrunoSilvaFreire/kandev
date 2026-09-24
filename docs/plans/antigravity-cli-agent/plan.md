---
spec: docs/specs/agents/requirements/antigravity-cli-agent.md
created: 2026-09-22
status: in_progress
---

# Implementation Plan: Antigravity CLI subscription support

Deliver one backend vertical slice: the `agy-acp` managed ACP bridge, native
`agy` passthrough, workspace MCP materialization, focused tests, and docs.
The official `antigravity-acp` kernel remains a separate agent.

## Work orders

- [Task 01](task-01-add-antigravity-cli-agent.md): agent declaration, MCP
  strategy, registry pin, documentation, deterministic checks, and local proof.

## Verification

Run targeted agent, MCP, registry, and lifecycle Go tests; then backend test and
lint, spec validation, and diff checks. Live proof uses `agy`, `acpdbg probe`,
and `acpdbg prompt` after deterministic checks pass.
