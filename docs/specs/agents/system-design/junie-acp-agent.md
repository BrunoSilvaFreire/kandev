---
status: draft
system: agents
requirements:
  - REQ-AGENTS-JUNIE-ACP-001
  - REQ-AGENTS-JUNIE-ACP-002
  - REQ-AGENTS-JUNIE-ACP-003
  - REQ-AGENTS-JUNIE-ACP-004
  - REQ-AGENTS-JUNIE-ACP-005
  - REQ-AGENTS-JUNIE-ACP-006
  - REQ-AGENTS-JUNIE-ACP-007
---

# JetBrains Junie ACP agent system design

## Purpose and boundaries

The agent system owns the Junie agent's declared identity, discovery predicate,
launch argument vector, config-root template, skill directories, remote-auth
methods, and install script. JetBrains' CLI owns its own credentials, session
storage, model catalogue, and update mechanism.

This design does not own ACP transport, process supervision, capability
probing, MCP wiring, or executor provisioning. It constrains only what Kandev
declares about this agent and what Kandev may assume about the CLI.

Values here were **measured** against `junie` version `26.6.29 (2144.7)`,
linux-x86_64, on 2026-09-26, unless a section says otherwise. The measured
values are a summary of that probing session (raw JSONL frames captured by
`acpdbg` and a raw JSON-RPC client). No other platform was executed. This is a
record of an external contract, not a Kandev implementation plan.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-AGENTS-JUNIE-ACP-001` | [Identity and discovery](#identity-and-discovery) |
| `REQ-AGENTS-JUNIE-ACP-002` | [Launch surface](#launch-surface) |
| `REQ-AGENTS-JUNIE-ACP-003` | [Persistence](#persistence) |
| `REQ-AGENTS-JUNIE-ACP-004` | [Security](#security) |
| `REQ-AGENTS-JUNIE-ACP-005` | [Capabilities and MCP](#capabilities-and-mcp) |
| `REQ-AGENTS-JUNIE-ACP-006` | [Distribution and provisioning](#distribution-and-provisioning) |
| `REQ-AGENTS-JUNIE-ACP-007` | [Control flow](#control-flow) |

## Components and responsibilities

- **Kandev agent declaration.** Identity, discovery, argument vector, config
  root, skill directories, remote-auth methods, install script. Static metadata
  only.
- **`junie`.** JetBrains' native CLI. Speaks ACP over stdio when launched with
  `--acp=true`, and also runs as an interactive or non-interactive task CLI
  without it. Self-updates through a launcher that applies a pending update under
  `<data>/junie/updates` before starting.

## Identity and discovery

The agent ID is `junie-acp`, matching the `<name>-acp` convention every native
ACP agent uses. Discovery is `Detect(ctx, WithCommand("junie"))`: a single PATH
lookup with no version or help probe. Junie is a real executable, unlike the
prose-valued detection some peers use, so a bare lookup is sufficient and keeps
discovery inert (no process start, no config read). The agent advertises
`SupportsMCP` and `SupportsSessionResume` when detected.

The ACP registry entry lists Junie, but this agent does not depend on the
registry: the operator installs the CLI (or uses the Settings card) and Kandev
launches the PATH binary.

The agent reports its own name as `@jetbrains/junie` and title `Junie` over ACP.
Kandev keeps its catalog name `Junie ACP Agent` and display name `Junie`,
consistent with the other native ACP agents.

## Launch surface

One argument vector serves the session command, the runtime command, and the
one-shot inference command, so no path can drift to a different one:

| Surface | Argument vector |
| --- | --- |
| `BuildCommand`, `Runtime().Cmd`, `InferenceConfig().Command` | `junie --acp=true` |
| `PassthroughCmd` | `junie` |

`--acp=<[true|false]>` is declared by the CLI's help output and is a single
argv token. Both `junie --acp=true` and `junie --acp true` completed
`initialize` in the probe; Kandev pins the `=` form because the help declares it
explicitly and a single token cannot be split by a launcher that does not
preserve argument boundaries.

The working directory is the task workspace, the environment map is empty with
nothing stripped, and resource limits are the shared defaults (4096 MB, 2.0 CPU,
one hour).

**Which layer the vector describes.** `CommandBuilder.BuildCommand` calls the
agent's own `BuildCommand` and then appends the profile's `cli_flags` to every
agent's vector, after which the sandbox prefix is prepended. No agent has an
opt-out, and adding one here would change a contract shared by all of them. So
the "exactly" in AC-AGENTS-JUNIE-ACP-002.1 binds this agent's own construction
only; `AC-AGENTS-JUNIE-ACP-002.9` states that binding.

## Persistence

Junie writes its state under `~/.junie`:

| Path | Contents |
| --- | --- |
| `<home>/.junie/sessions/<session-id>/events.jsonl` | session event stream |
| `<home>/.junie/sessions/<session-id>/state.json` | session state |
| `<home>/.junie/sessions/index.jsonl` | session index |
| `<home>/.junie/settings.json` | settings |
| `<home>/.junie/secure_credentials.json` | stored credentials |
| `<home>/.junie/AGENTS.md` | user guidelines |
| `<home>/.junie/skills/` (and `~/.agents/skills`) | skills |

Measured: a prompt created `~/.junie/sessions/session-260926-133044-1piv/`
holding `events.jsonl` and `state.json`. The `~/.local/share/junie` tree holds
the installed binary, shim, and updates (`current`, `updates`, `versions`), not
session state. The declared template is therefore `{home}/.junie` and the
container target is `/root/.junie`.

`SessionDirTemplate` is a static string on every agent and is consumed by the
container bind-mount source. It is accurate exactly when `JUNIE_HOME` is unset
or empty; a non-default `JUNIE_HOME` is a named exclusion in the requirements.

**Resume and workspace rebind.** `initialize` advertises `loadSession: true`
and `sessionCapabilities` including `resume` and `list`, so native session
resume and recovery are declared. `session/load` behavior is
working-directory-sensitive, measured deterministically over two runs each:

| Working directory | `session/load` result |
| --- | --- |
| Unchanged (the session's project directory) | full model config, 16 model options |
| Changed to an unrelated directory | result without a model config or session id |

Junie scopes a session to its project directory, so a rebind does not restore
the session. The agent therefore declares
`NewSessionOnWorkspaceRebind: true` (force `session/new` on a cwd change) and is
added to `workspace_rebind_resume_test.go` as a fresh-session agent. It is
deliberately not in the "loads session in changed cwd" group.

## Capabilities and MCP

`initialize` (protocol v1) returned:

- `agentInfo`: `{name: @jetbrains/junie, title: Junie, version: 26.6.29 (2144.7)}`
- `agentCapabilities`: `loadSession: true`; `promptCapabilities`
  `{image: true, audio: false, embeddedContext: true}`; `mcpCapabilities`
  `{http: true, sse: true}`; `sessionCapabilities` `{additionalDirectories, fork,
  list, resume}`; `auth: {logout}`
- `authMethods`: `jetbrains-account` (type `agent`), `junie-cli` (type
  `terminal` with a `terminal-auth` command whose env sets `JUNIE_HOME`)

`session/new` initially returned `configOptions` with ids `effort` and
`brave_mode`; Junie advertised `model` (16 options) in a later
`config_option_update`, measured about two seconds after session creation.
The Junie-only capability probe therefore waits up to five seconds for that
model advertisement; other agents retain the immediate probe path. The final
option set contains ids `model`,
`effort` (`low`/`medium`/`high`/`xhigh`/`max`), and `brave_mode` (boolean). No
`modes` were advertised, which is the shared "agent exposes no mode surface"
case rather than a defect. Model/mode state flows through the existing config
option refresh.

MCP servers are passed through `session/new`; the agent declares no project MCP
strategy. A raw probe confirmed `session/new` accepts a `stdio` server, an
`http` server, and an `sse` server. One strictness worth recording: Junie's
deserializer requires the `headers` field to be present on `http` transports.
Kandev's ACP conversion always emits `"headers":[]` for http and sse servers
(`mapToHTTPHeaders` returns a non-nil empty slice, and the SDK field is not
`omitempty`), so Kandev's payload is accepted. `acpdbg mcp-probe`'s sentinel
omitted the field and was rejected; that is a probe-tool artifact, not a Kandev
incompatibility, and no `acpcompat` rule is needed.

`skill` resolution read user skills from `~/.agents/skills` (measured). Kandev
declares the project skill directory `.agents/skills` (the shared fallback) and
the user skill directory `.agents/skills`.

## Distribution and provisioning

Junie is a standalone native binary, not an npm package. The Settings card's
install action downloads the official installer to a temporary file and runs
it, then adds `~/.local/bin` to PATH and verifies `command -v junie`. The
official installer extracts a platform archive, so `unzip` is a prerequisite;
this is stated in the agent description. A failed download fails the script, so
a remote bootstrap cannot silently succeed.

`junie --version` applied a pending update through its launcher before printing
the version, so the CLI self-updates. No update activity was observed on stderr
during the measured ACP handshake, and the CLI exposes a `--skip-update-check`
flag and `JUNIE_SKIP_UPDATE_CHECK` environment variable. The declared vector
stays minimal; a follow-up adds `--skip-update-check` only if a measured launch
shows the update check delaying or breaking `initialize`.

## Security

Credentials are written and read by the CLI alone. Kandev passes the
`JUNIE_API_KEY` environment variable for remote executors and otherwise does not
create, modify, or delete anything under the config root.

Non-interactive use is authenticated either by `JUNIE_API_KEY` or by the CLI's
`--auth` token flag; the stored credential lives in
`~/.junie/secure_credentials.json`. Measured without a key: `session/new`
succeeded, and the first prompt returned
`401 Unauthorized: Missing required header: Authorization`, so the
missing-credential state surfaces at prompt time through the shared ACP path.
The agent advertises `jetbrains-account` (browser) and `junie-cli` (terminal)
login methods; Kandev presents those in the returned order and declares no
`LoginAgent` because the CLI has no documented non-interactive login command.

## Control flow

Kandev spawns one `junie` process per session and speaks ACP over its stdio.

Concurrent sessions run independent processes over a shared config root. Kandev
adds no lock: arbitration inside the config root is the CLI's own concern.

## Observability

The CLI logs to `<home>/.junie/logs/log-<timestamp>-pid-<pid>.log`. The two
lines worth recognising are the update line emitted by the launcher on startup
and the `401 Unauthorized` failure when no credential is configured. The agent
emits no new metrics; routing and stall telemetry are owned by the shared
layers.

## Deferred work

**A non-default `JUNIE_HOME`.** Supporting it means making the declared
template environment-derived, which the static `SessionDirTemplate` contract
shared by every agent does not allow today.

**Non-interactive login.** Adding a `LoginAgent` requires JetBrains to document
a non-interactive login command; today only `--auth <token>` and the two
advertised interactive methods exist.

**Cross-platform verification.** Only linux-x86_64 was executed. The binary
name and argument vector are platform-independent by inspection, but the
installer and the update launcher were not exercised on macOS or Windows.

**Update-check suppression.** `--skip-update-check` is available but not passed;
add it if a future measured launch shows the update check interfering with ACP
startup.

## Related decisions

None. This design records an external vendor contract and introduces no new
Kandev architecture boundary.
