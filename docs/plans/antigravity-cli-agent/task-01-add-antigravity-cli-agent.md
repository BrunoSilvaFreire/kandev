---
plan: docs/plans/antigravity-cli-agent/plan.md
status: in_progress
---

# Task 01: Add Antigravity CLI agent

## Outcome

Expose the existing local Antigravity subscription through pinned `agy-acp`
without changing the official Antigravity kernel.

## Scope

Implement `AgyACP`, `AntigravityStrategy`, registry/package registration,
contract tests, and supported-boundary documentation. Remote credentials,
frontend work, and global config mutation are excluded.

## Requirements and design

- `REQ-AGENTS-ANTIGRAVITY-CLI-001..004`
- [System design](../../specs/agents/system-design/antigravity-cli-agent.md)

## Acceptance and verification

- `agy-acp` is detected through `agy --version`, runs the exact managed bridge,
  and has native passthrough and local-only auth.
- `.agents/mcp_config.json` merges the Kandev MCP server without deleting user
  entries.
- Run `go test ./internal/agent/agents ./internal/agent/mcpconfig
  ./internal/agent/registry ./internal/agent/runtime/lifecycle -count=1`, backend
  test/lint, and the documentation validators.

## Results

Implementation is present. On 2026-09-22, host `agy --version` returned 1.2.8;
`agy models` included Claude Sonnet 4.6, Claude Opus 4.6, and GPT-OSS 120B.
The exact pinned bridge answered ACP `initialize` as `agy-acp` 0.5.2 with
session load/resume and model configuration support. Backend Go tests, build,
lint, `acpdbg prompt`, Kandev MCP delivery, and resume remain blocked because
this environment has no `go` executable.
