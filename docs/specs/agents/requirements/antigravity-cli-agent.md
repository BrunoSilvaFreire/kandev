---
status: draft
system: agents
implementation_plans:
  - docs/plans/antigravity-cli-agent/plan.md
requirements:
  - REQ-AGENTS-ANTIGRAVITY-CLI-001
  - REQ-AGENTS-ANTIGRAVITY-CLI-002
  - REQ-AGENTS-ANTIGRAVITY-CLI-003
  - REQ-AGENTS-ANTIGRAVITY-CLI-004
---

# Antigravity CLI ACP agent requirements

### REQ-AGENTS-ANTIGRAVITY-CLI-001: Separate local agent identity

Kandev shall expose `agy-acp` as **Antigravity CLI**, separately from Google's
official `antigravity-acp` kernel. Discovery shall require `agy --version` on
the backend host. The agent is subscription billed, uses the existing local
`agy` login, and is not auto-installed.

#### Acceptance criteria

- **AC-AGENTS-ANTIGRAVITY-CLI-001.1:** `TestAgyACP` and the local `agy --version`
  check prove the separate identity and host discovery.

### REQ-AGENTS-ANTIGRAVITY-CLI-002: Command and session boundary

Structured sessions and inference shall run exactly `npx --yes --prefer-offline
agy-acp@0.5.2`; passthrough shall run the installed `agy` binary. Models, modes,
and effort shall come from ACP discovery. The bridge's persisted state and agy's
conversation store shall support native resume.

#### Acceptance criteria

- **AC-AGENTS-ANTIGRAVITY-CLI-002.1:** Managed-runtime and agent contract tests,
  plus `acpdbg prompt` and resume checks, prove the command and session boundary.

### REQ-AGENTS-ANTIGRAVITY-CLI-003: Local-only auth and risk disclosure

Login shall open local `agy` in a PTY. Remote auth shall be nil: Kandev shall
not export OS-keyring credentials to remote or container executors. Product
documentation shall disclose that this third-party bridge can violate Google
terms and risk account suspension.

#### Acceptance criteria

- **AC-AGENTS-ANTIGRAVITY-CLI-003.1:** `TestAgyACP` and documentation review
  prove local-only auth and the risk disclosure.

### REQ-AGENTS-ANTIGRAVITY-CLI-004: Workspace MCP delivery

Because the bridge does not forward ACP `session/new.mcpServers`, Kandev shall
merge resolved servers into workspace-local `.agents/mcp_config.json` under
`mcpServers`. Existing entries survive; Kandev wins only same-name collisions.
No global Gemini config is modified.

#### Acceptance criteria

- **AC-AGENTS-ANTIGRAVITY-CLI-004.1:** `TestAntigravityStrategy` and
  `TestMaterializeRuntimeProjectMCPForAgy` prove project-local merged delivery.
