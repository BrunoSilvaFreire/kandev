---
status: draft
system: agents
requirements:
  - REQ-AGENTS-ANTIGRAVITY-CLI-001
  - REQ-AGENTS-ANTIGRAVITY-CLI-002
  - REQ-AGENTS-ANTIGRAVITY-CLI-003
  - REQ-AGENTS-ANTIGRAVITY-CLI-004
implementation_plans:
  - docs/plans/antigravity-cli-agent/plan.md
---

# Antigravity CLI ACP agent system design

`agy` and `agy-acp` are distinct artifacts. `agy` is the native, logged-in CLI
on the local host; `agy-acp@0.5.2` is the managed third-party stdio ACP bridge.
The bridge starts `agy`; Kandev does not install either native CLI or copy its
keyring-backed credentials. This deliberately differs from the official
`antigravity-acp` kernel, whose credentials and session contract remain intact.

The runtime command and inference command are `npx --yes --prefer-offline
agy-acp@0.5.2`; raw passthrough is `agy --model <model>`. Bridge state lives in
`~/.agy-acp-state` and native conversations under `~/.gemini/antigravity-cli`.
The existing isolated-home mount (`{home}` to `/root`) preserves both for local
sessions. Remote auth is intentionally nil.

The bridge does not relay ACP MCP servers. Before launch, `AntigravityStrategy`
serializes them at `.agents/mcp_config.json` as `{ "mcpServers": { ... } }`.
Stdio entries carry command, args, and env; remote entries carry `serverUrl`
and headers. The lifecycle merge retains unrelated user settings and user server
names unless Kandev supplies the same name.

Failure to find or execute `agy --version` hides the agent. Failure to write the
workspace MCP file fails launch; Kandev never falls back to global config.
