---
spec: docs/specs/agents/requirements/junie-acp-agent.md
created: 2026-09-26
status: complete
---

# Implementation Plan: JetBrains Junie ACP agent

Deliver one backend vertical slice: the native `junie-acp` agent declaration,
logos, registry wiring, focused tests, and docs. No managed npm runtime, no
frontend change, no i18n keys.

## Work orders

- [Task 01](task-01-add-junie-acp-agent.md): agent declaration, registry pin,
  tests, documentation, deterministic checks, and local proof.

## Verification

Run targeted agent and registry Go tests; then backend test and lint, spec
validation, and diff checks. Live proof uses the installed `junie` binary,
`acpdbg probe`, `session/load`, and a raw `session/new` MCP probe.
