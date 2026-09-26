---
status: draft
system: agents
created: 2026-09-26
owners:
  - kandev
---

# JetBrains Junie ACP agent requirements

## Overview

JetBrains publishes Junie as a native CLI that speaks ACP over stdio in
`--acp` mode. Kandev needs it as a built-in agent so a user can select it for a
task, run and resume a session, and use MCP servers and skills, without
inventing a distribution, credential, or session-storage mechanism JetBrains
already owns. The agent system owns the durable decision: an agent identity plus
its discovery, launch, config-root, authentication, and capability contract;
transport and process supervision belong to the runtime. This layer assumes the
operator installed Junie; automated provisioning is named as a supported install
action, and the remaining executor exclusions are stated explicitly rather than
left silent.

## Terminology

- **Junie:** The JetBrains AI coding CLI, launched as the `junie` binary. Its
  ACP mode is enabled with `--acp=true`.
- **Junie home:** `~/.junie`, the directory the CLI writes for configuration,
  credentials, user skills, and session state. The CLI also honors a
  `JUNIE_HOME` environment override for its login flow. Holds sessions under
  `<home>/sessions/<session-id>/`. What Kandev declares about it is
  REQ-AGENTS-JUNIE-ACP-003.
- **Advertised capabilities:** The `agentCapabilities` and `authMethods` in the
  ACP `initialize` result.

## Evidence

Values are of two kinds. **Measured** ones come from a manual ACP probing
session against `junie` version `26.6.29 (2144.7)`, linux-x86_64, on
2026-09-26, using `acpdbg` and a raw JSON-RPC client; the binary was already
installed on the probe host. **Not measured** values are marked as such in the
system design. The probe created two throwaway sessions under the probe host's
`~/.junie/sessions`, which is the CLI's own store.

## Requirements

### REQ-AGENTS-JUNIE-ACP-001: Built-in Junie agent identity and discovery

**Intent:** Give Junie a stable catalog identity and report it available only
when its executable is present.

#### Acceptance criteria

- **AC-AGENTS-JUNIE-ACP-001.1:** The agent identifier shall be `junie-acp`,
  following the `<name>-acp` convention every native ACP agent uses, with
  display name `Junie` and catalog name `Junie ACP Agent`.
- **AC-AGENTS-JUNIE-ACP-001.2:** The agent shall be enabled by default and
  present in the default registry.
- **AC-AGENTS-JUNIE-ACP-001.3:** The display order shall be `25`, distinct from
  every other built-in agent's, so the display-order sort never reaches its
  stable-sort tiebreak here.
- **AC-AGENTS-JUNIE-ACP-001.4:** The agent shall return a non-empty light and
  dark logo, and the light one for any other variant.
- **AC-AGENTS-JUNIE-ACP-001.5:** Discovery shall perform exactly one PATH lookup
  for the binary name `junie`. When it is absent, discovery shall report
  unavailable with an empty matched path and no error; when present, the matched
  path shall be the path that lookup returned.
- **AC-AGENTS-JUNIE-ACP-001.6:** When discovery reports the agent available, MCP
  support and session resume shall both be reported as supported.
- **AC-AGENTS-JUNIE-ACP-001.7:** Discovery shall create no files, write no
  configuration, and start no process, so repeated calls over an unchanged PATH
  and filesystem return equal results.
- **AC-AGENTS-JUNIE-ACP-001.8:** When the caller's context is cancelled or
  expires during discovery, discovery shall return that error rather than
  reporting the agent as unavailable.

### REQ-AGENTS-JUNIE-ACP-002: One launch command across every surface

**Intent:** Keep the session, runtime, and inference commands identical, so a
session and a capability probe cannot disagree about how the agent starts.

#### Acceptance criteria

- **AC-AGENTS-JUNIE-ACP-002.1:** The argument vector shall be exactly
  `junie --acp=true`, where `--acp=true` is a single argv token.
- **AC-AGENTS-JUNIE-ACP-002.2:** Command construction shall be deterministic:
  repeated calls produce equal vectors, consulting neither PATH, the filesystem,
  nor the environment.
- **AC-AGENTS-JUNIE-ACP-002.3:** The session, runtime, and one-shot inference
  commands shall produce the same argument vector.
- **AC-AGENTS-JUNIE-ACP-002.4:** The argument vector shall not vary with the
  selected model, permission policy, auto-approve setting, agent type, or resume
  target.
- **AC-AGENTS-JUNIE-ACP-002.5:** CLI passthrough shall launch the bare `junie`
  binary with no arguments.
- **AC-AGENTS-JUNIE-ACP-002.6:** The working directory shall be the task
  workspace.
- **AC-AGENTS-JUNIE-ACP-002.7:** The runtime shall declare the ACP protocol, an
  empty environment map, and no stripped variables, so the agent inherits the
  execution environment unmodified.
- **AC-AGENTS-JUNIE-ACP-002.8:** Resource limits shall be 4096 MB memory, 2.0
  CPU cores, and a one-hour timeout, matching other built-in ACP agents.
- **AC-AGENTS-JUNIE-ACP-002.9:** AC-AGENTS-JUNIE-ACP-002.1 through `.5` bind
  this agent's own command construction, not the final launched argv: the shared
  builder appends the profile's `cli_flags` and any sandbox command prefix to
  every agent afterwards, and this agent shall declare no filter for either.
- **AC-AGENTS-JUNIE-ACP-002.10:** This agent's own construction shall not add
  `--skip-update-check`, telemetry flags, a model flag, or a permission flag.
  The startup update check did not appear in the measured ACP handshake, so the
  declared vector stays minimal; a follow-up may add `--skip-update-check` only
  if a measured launch shows the check delaying or breaking `initialize`.

### REQ-AGENTS-JUNIE-ACP-003: Persistence, sessions, and skills

**Intent:** Point Kandev at the directories the CLI uses, so a long task
survives a relaunch and Kandev never writes state it does not own.

#### Acceptance criteria

- **AC-AGENTS-JUNIE-ACP-003.1:** The session directory template shall be
  `{home}/.junie`, the directory under which the CLI writes
  `sessions/<session-id>/`. Measured.
- **AC-AGENTS-JUNIE-ACP-003.2:** The container session-directory target shall be
  `/root/.junie`, so the declared template resolves to a bind mount under the
  container root home.
- **AC-AGENTS-JUNIE-ACP-003.3:** The agent shall declare native session resume
  and that a session can be recovered. Measured: `initialize` advertises
  `loadSession: true` and `sessionCapabilities` including `resume` and `list`,
  and `session/load` with an unchanged working directory returned the full model
  configuration.
- **AC-AGENTS-JUNIE-ACP-003.4:** The agent shall force `session/new` when an idle
  execution's working directory changes. Measured: `session/load` with a
  changed working directory returned a result without the session's model
  configuration and without a session identifier, so the session's state is
  scoped to the project directory and does not survive a rebind.
- **AC-AGENTS-JUNIE-ACP-003.5:** The project skill directory shall be
  `.agents/skills`, the shared fallback every agent uses.
- **AC-AGENTS-JUNIE-ACP-003.6:** The user skill directory shall be
  `.agents/skills`, the home-relative directory the CLI reported loading user
  skills from. Measured.
- **AC-AGENTS-JUNIE-ACP-003.7:** During a session, Kandev shall not create,
  modify, or delete any file under the config root. Its only write there is the
  user-initiated remote-auth environment handoff in
  REQ-AGENTS-JUNIE-ACP-004, which seeds a remote host before a session starts.

### REQ-AGENTS-JUNIE-ACP-004: Authentication owned by Junie

**Intent:** Surface the CLI's own authentication contract instead of duplicating
it, so an unauthenticated agent reads as "needs login", not "broken".

#### Acceptance criteria

- **AC-AGENTS-JUNIE-ACP-004.1:** Kandev shall present the authentication methods
  the agent advertises in its `initialize` result rather than a hardcoded list,
  in the order returned. Measured: `jetbrains-account` (type `agent`) and
  `junie-cli` (type `terminal`, carrying a `terminal-auth` command whose
  environment sets `JUNIE_HOME`).
- **AC-AGENTS-JUNIE-ACP-004.2:** This agent shall declare no auth classification
  of its own, relying on the shared ACP handling of an authentication-required
  error from a capability probe or session start.
- **AC-AGENTS-JUNIE-ACP-004.3:** When a prompt is sent without valid
  credentials, Kandev shall surface the CLI's own failure. Measured: the agent
  reported `401 Unauthorized: Missing required header: Authorization` after
  `session/new` succeeded, so the missing-credential state is reported at prompt
  time.
- **AC-AGENTS-JUNIE-ACP-004.4:** Remote authentication shall offer one
  environment-variable method for `JUNIE_API_KEY`, the key the CLI documents for
  non-interactive use (`--auth` is its command-line equivalent). A setup hint
  shall point at `https://junie.jetbrains.com/cli`.
- **AC-AGENTS-JUNIE-ACP-004.5:** Kandev shall not mint, refresh, or rewrite
  credentials; it only passes the user-supplied environment variable and, for
  the terminal method, lets the CLI run its own login flow. No `LoginAgent` is
  declared because the CLI exposes no documented non-interactive login command.

### REQ-AGENTS-JUNIE-ACP-005: Capabilities and MCP read from the agent

**Intent:** Derive behavior from what the agent advertises, so an echoed
protocol version is never taken for a capability claim.

#### Acceptance criteria

- **AC-AGENTS-JUNIE-ACP-005.1:** This agent shall add no
  `protocolVersion`-derived branch to the shared ACP capability path, which
  already reads the advertised `agentCapabilities`; the echoed
  `protocolVersion` carries no capability information.
- **AC-AGENTS-JUNIE-ACP-005.2:** MCP servers shall be passed through ACP
  `session/new`, and the agent shall not declare a project-local MCP
  configuration strategy. Measured: `session/new` accepted a `stdio` server, an
  `http` server with an empty `headers` list, and an `sse` server.
- **AC-AGENTS-JUNIE-ACP-005.3:** The agent shall not declare an assumed HTTP or
  SSE MCP override, because the agent advertises both transports
  (`mcpCapabilities: {http: true, sse: true}`).
- **AC-AGENTS-JUNIE-ACP-005.4:** The agent shall not declare that it namespaces
  MCP tool names by server.
- **AC-AGENTS-JUNIE-ACP-005.5:** Billing type shall be resolved by the shared
  default rule rather than pinned by this agent.
- **AC-AGENTS-JUNIE-ACP-005.6:** The agent shall declare no `acpcompat`,
  dialect, or stderr-derived failure rule unless a measured probe or smoke test
  shows a quirk. The measured handshake needed none.

### REQ-AGENTS-JUNIE-ACP-006: Automated provisioning of the CLI

**Intent:** Let a user install Junie from the Settings card with the official
installer, and fail loudly when the download fails.

#### Acceptance criteria

- **AC-AGENTS-JUNIE-ACP-006.1:** The install script shall run the official
  `https://junie.jetbrains.com/install.sh` installer.
- **AC-AGENTS-JUNIE-ACP-006.2:** The install script shall download the installer
  to a temporary file and run it from there, so a failed download cannot be
  hidden by a successful shell exit status, and shall remove the temporary file
  on exit.
- **AC-AGENTS-JUNIE-ACP-006.3:** After installing, the install script shall add
  `~/.local/bin` to PATH for the remainder of the script and shall verify
  `command -v junie` succeeds.
- **AC-AGENTS-JUNIE-ACP-006.4:** The agent metadata (description, or the script
  itself where it is runnable shell) shall name the `unzip` prerequisite, because
  the official installer extracts a platform archive.
- **AC-AGENTS-JUNIE-ACP-006.5:** A failed download shall fail the install rather
  than reporting success.

### REQ-AGENTS-JUNIE-ACP-007: Concurrent sessions

**Intent:** State that concurrency in the config root belongs to the CLI, so two
tasks run at once and no one adds a lock the CLI does not expect.

#### Acceptance criteria

- **AC-AGENTS-JUNIE-ACP-007.1:** Two concurrent Kandev sessions using this agent
  shall each launch an independent `junie` process.
- **AC-AGENTS-JUNIE-ACP-007.2:** Kandev shall not serialize those processes and
  shall not lock, guard, or arbitrate access to the shared config root;
  conflicting writes there are the CLI's responsibility.
- **AC-AGENTS-JUNIE-ACP-007.3:** Terminating one session shall not terminate the
  other nor remove shared state under the config root.

## Out of scope

Each exclusion is a contract, not silence.

- **A non-default `JUNIE_HOME`.** The CLI honors a `JUNIE_HOME` override for its
  login flow and terminal-auth method. Kandev declares the default
  `{home}/.junie` template and never reads the variable, so session-dir seeding
  and skills address the default root only. This layer neither detects nor
  reconciles a non-default root.
- **Non-interactive login.** The CLI has no documented non-interactive login
  command, so the agent declares no `LoginAgent`; the two advertised auth
  methods run their own flows.
- **Model catalog curation.** Models come from the existing ACP probe
  (`configOptions` id `model`, 16 entries measured). No static model list, no
  model flag.
- **Windows, macOS, and Bash-on-Windows verification.** Only linux-x86_64 was
  measured. The binary name and argument vector are platform-independent by
  inspection, but the installer's platform behavior is not measured here.
- **Office routing eligibility.** The Office v1 provider list is intentionally
  closed and this change does not touch it.
