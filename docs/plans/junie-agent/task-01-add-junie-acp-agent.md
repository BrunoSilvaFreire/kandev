---
plan: docs/plans/junie-agent/plan.md
status: done
---

# Task 01: Add Junie ACP agent

## Outcome

Expose JetBrains Junie as a native ACP agent (`junie-acp`) that discovers the
`junie` binary, launches `junie --acp=true`, resumes its project-scoped
sessions when the workspace is unchanged, and accepts MCP servers through ACP
`session/new`.

## Scope

Implement `JunieACP`, register it in `LoadDefaults()`, add light/dark logos,
contract tests, and supported-boundary documentation. Managed npm runtime,
frontend work, and Office routing eligibility are excluded.

## Requirements and design

- `REQ-AGENTS-JUNIE-ACP-001..007`
- [System design](../../specs/agents/system-design/junie-acp-agent.md)

## Acceptance and verification

- `junie-acp` is detected through a `junie` PATH lookup, runs
  `junie --acp=true` on every command surface, has native passthrough, resolves
  `JUNIE_API_KEY` for remote auth, and reports the `{home}/.junie` session
  directory.
- Run `go test ./internal/agent/agents ./internal/agent/registry
  ./internal/agent/runtime/lifecycle ./internal/agentctl/server/adapter/...
  -count=1`, backend test/lint, and the documentation validators.

## Results

Gate G1 was measured on 2026-09-26 against `junie` 26.6.29 (2144.7),
linux-x86_64: `junie --acp=true` completed `initialize` and `session/new`
(protocol v1, 16 models, config option ids `model`/`effort`/`brave_mode`, no
modes). Session state is `~/.junie/sessions/<id>/`. `session/load` restores the
session only in the unchanged project directory, so the agent forces a new
session on workspace rebind. A raw `session/new` accepted stdio, http, and sse
MCP servers using Kandev's payload shape. No credential was present, so the
first prompt reported `401 Unauthorized: Missing required header:
Authorization`, confirming the shared auth-required surfacing path.

Implementation, tests, and docs are present in the working tree.
