# Local Bootstrap (fork)

Reproducible steps used to bring this fork up on Linux (Debian 13, x86_64, bash). Validated 2026-09-22 against upstream `4f76997c8`.

## 1. Clone

```bash
cd ~/MiscProjects
gh repo fork kdlbs/kandev --clone      # or: git clone git@github.com:BrunoSilvaFreire/kandev.git
cd kandev
git remote set-url upstream https://github.com/kdlbs/kandev
git remote set-url --push upstream DISABLED
```

See `docs/upstream-strategy.md` for remotes and syncing.

## 2. Toolchain and dependencies

Pinned in `mise.toml`: Go 1.26.0, Node 24, pnpm 9.15.9, jq, ripgrep, uv, golangci-lint 2.9.0, git-cliff, go-licenses, pre-commit.

```bash
make bootstrap
# == scripts/bootstrap-dev-env: installs mise if missing (to ~/.local/bin),
#    appends `eval "$(mise activate bash)"` to ~/.bashrc, runs `mise install`,
#    `pnpm install --frozen-lockfile` in apps/, and wires pre-commit hooks.
```

Notes:

- The script tries `sudo apt-get install ...` for OS packages. Without passwordless sudo, run it as
  `scripts/bootstrap-dev-env --without-os-packages` and check the packages yourself:
  `bash build-essential ca-certificates curl file git gzip lsof make openssh-client pipx pkg-config python3 python3-pip rsync sqlite3 tar unzip xz-utils zip libsqlite3-dev`.
  On this machine only the `sqlite3` CLI was missing (optional; the backend links SQLite via cgo).
- New shells pick up the mise toolchain through `~/.bashrc`. Non-interactive scripts need
  `eval "$(mise env -s bash)"` (or `mise exec -- <cmd>`).
- A system Node (e.g. nvm) can coexist; mise's Node 24 comes first on `PATH` inside the repo.

## 3. Agent CLIs (host)

KanDev discovers CLIs on the backend process `PATH`. Installed globally (nvm Node) on this machine:

```bash
npm install -g @openai/codex opencode-ai @google/gemini-cli
curl -fsSL https://claude.ai/install.sh | bash     # Claude Code -> ~/.local/bin/claude
```

Authentication is per-CLI and reused by KanDev. See `docs/provider-setup.md`.

## 4. Run the dev stack

```bash
make dev
```

- Builds `apps/backend/bin/kandev`, starts the backend on **:38429** (serves UI + API + `/mcp`) and Vite on **:37429**.
- Dev profile (`profiles.yaml`): Office **on**, mock agent registered alongside real CLIs, pprof/debug on, ACP frame logs on.
- Data lives in `./.kandev-dev/` (gitignored): SQLite DB `data/kandev.db`, task worktrees `tasks/`, ACP frame logs `logs/acp/`.
- Health: `curl localhost:38429/health` → `{"status":"ok",...}`. UI: <http://localhost:38429>.
- **Binds 0.0.0.0** (both ports are printed with LAN addresses) and auth is off by default. Firewall it or use a trusted network.

### Launching from inside another agent session

If KanDev is launched from a shell that is itself inside Claude Code (e.g. the desktop app), that shell carries
`CLAUDECODE`, `ANTHROPIC_BASE_URL`, `CLAUDE_CODE_*` variables which KanDev passes to every agent it spawns. Strip them:

```bash
env -i HOME="$HOME" USER="$USER" SHELL=/bin/bash TERM=xterm-256color PATH="$HOME/.local/bin:$PATH" \
  XDG_RUNTIME_DIR="$XDG_RUNTIME_DIR" SSH_AUTH_SOCK="$SSH_AUTH_SOCK" DBUS_SESSION_BUS_ADDRESS="$DBUS_SESSION_BUS_ADDRESS" \
  DISPLAY="$DISPLAY" XAUTHORITY="$XAUTHORITY" WAYLAND_DISPLAY="$WAYLAND_DISPLAY" \
  bash -lc 'cd ~/MiscProjects/kandev && eval "$(mise env -s bash)" && make dev'
```

Keep the display variables: agents inherit the backend's environment, so without `DISPLAY`/`XAUTHORITY` any headful
work an agent starts (e.g. a Minecraft client for integration runs) fails with "Authorization required". Changing the
backend environment requires a KanDev restart; running agents are interrupted and must be resumed by sending a message
(sessions come back as `WAITING_FOR_INPUT`, the workflow step is preserved).

## 5. Production/native launch path

```bash
make start      # production build (web embedded in the Go binary), prod profile, data in ~/.kandev
```

Prod profile differences: Office off, no mock agent, no debug endpoints. Toggle experimental features under
**Settings > System > Feature Toggles** (restart required).

## 6. Smoke checklist

1. `curl localhost:38429/health` is ok.
2. UI loads; the first **New Task** opens an onboarding wizard (agents, executors, workflows, command panel).
3. **Settings > Workspaces > _ws_ > Repositories > Add Local Repository** accepts a local git path.
4. `curl localhost:38429/api/v1/agents/discovery` lists installed CLIs as `available: true`.
5. New Task → agent runs in a worktree under `.kandev-dev/tasks/…`, Changes panel shows the diff, task moves to Review.

## Known dev-environment issues

- `mock-agent` shows **Error** (`exec: "mock-agent": executable file not found`): `make dev` builds only `kandev`.
  Run `make -C apps/backend build` once if you want the mock agent.
- A repository without an `origin` remote logs a harmless "Remote refresh was incomplete" warning at worktree creation.
- Settings-mutating REST endpoints need the per-boot `X-Kandev-Interim-Settings-Interlock` header; scripts can read it from
  `GET /api/v1/app-state?path=%2Fsettings%2Fagents` (`interimSettingsInterlockToken`).
