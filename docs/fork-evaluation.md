# Fork Evaluation (initial bootstrap)

Findings from bootstrapping upstream `4f76997c8` (v0.95.1-8) on 2026-09-22 and running real subscription-backed agents.
Status labels: **Works** (exercised end to end here), **Implemented** (code + docs, not exercised), **Flagged** (behind a runtime
flag / Office), **Missing**.

## What works well

| Area | Status | Evidence |
|---|---|---|
| Bootstrap (`make bootstrap`, `make dev`) | Works | Clean tree after bootstrap; backend healthy on :38429, Vite on :37429 (`docs/local-bootstrap.md`). |
| CLI discovery | Works | Claude, Codex, OpenCode, Gemini all `available: true` from host `PATH`. |
| Subscription auth reuse | Works (Claude, Codex, OpenCode) | Claude ACP stream shows claude.ai unified rate-limit windows, no overage (`docs/provider-setup.md`). |
| Task → worktree → agent → review | Works | Claude smoke task in a sandbox repo: worktree under `.kandev-dev/tasks/`, permission prompts, Changes panel, auto-move to Review. |
| Local executor on a dirty repo | Works | Mineflayer run: agent operates in the real checkout; 184 pre-existing uncommitted entries preserved. |
| Per-step agent profiles + auto-advance | Works | Role Pipeline workflow (below): OpenCode → Codex handoff with `step_complete_kandev` gating. |
| Task plan as a cross-provider artifact | Works | Spike wrote a 20 KB plan via `create_task_plan_kandev`; the next step's Codex agent read it via `get_task_plan_kandev`. |
| Workflow YAML import with explicit profile bindings | Works | `POST /workspaces/:id/workflows/import` (preview → bindings). |
| ACP frame logging (dev) | Works | `.kandev-dev/logs/acp/{raw,normalized}-*.jsonl` — invaluable for billing/quota forensics. |

## Multi-provider workflow (Role Pipeline)

Workflow `Role Pipeline` in workspace `mineflayer`. Portable definition: `docs/examples/role-pipeline.workflow.yml`
(import via **Settings > Workspaces > Workflows > Import**; profiles match by agent + model, see `docs/provider-setup.md`).

`Backlog → Spike (OpenCode deepseek-v4.1-flash) → Architect (Codex gpt-5.6-sol) → Plan Approval (human) → Implement (Codex gpt-5.6-terra) → Review (Claude opus) → Done`

Observed behavior:

- **Context transfer:** only what you put in durable Kandev artifacts. Each step with a different profile starts a **new provider
  session** (`profile_session_start_policy: new`); provider chat history is not carried. The task **plan** (`task_plans`), the step
  prompt, the original task description (`{{task_prompt}}`), and `get_task_conversation_kandev` are the handoff channels.
- **Shared workspace:** all steps share the same task environment (here: the Local executor's real checkout). No per-step worktree.
- **Gating:** `auto_advance_requires_signal: true` + the agent calling `step_complete_kandev` is what moves the task; a plain
  turn end does not. Without it, any turn end (including a clarifying question) would advance.
- **Artifacts:** the Spike writes task document `spike` (not the plan); only the Architect writes the task plan, and each
  new plan is a forced plan revision (`new_revision=true`), so superseded plans stay readable by revision number.
- **Conditional approval:** the first plan always goes Architect → `Plan Approval` (agent-less human gate) via
  `step_complete_kandev` + `move_to_next`. After an Implementer escalation (`move_task_kandev` to Architect with
  `ESCALATION: ...`), the Architect returns straight to Implement with `move_task_kandev` and no `step_complete_kandev`;
  it parks at Plan Approval only when it signals completion with `APPROVAL REQUESTED: <reason>`. The first-plan rule is
  prompt-enforced, not engine-enforced.
- **Per-step profile:** yes; stored as `workflow_steps.agent_profile_id`. The New Task dialog also shows the chosen profile.
- **Agent skill pickup:** each CLI loads its own user-level skills (`~/.config/opencode/skills`, `~/.agents/skills`, `~/.claude`),
  so personal skills (e.g. `cityscape-extract-resume-handoff`) work inside Kandev without configuration.
- Results of the Implement/Review steps: _see "Run log" at the end of this file._

## Subscription / quota visibility — current state

| Capability | Status | Where |
|---|---|---|
| Billing-type detection (subscription vs API key) | Implemented for Claude + Codex only; everything else hard-coded `api_key` | `apps/backend/internal/agent/agents/billing.go`, `BillingType()` on each agent |
| Live quota windows (5h/7d, reset time) via provider usage APIs | Implemented for Claude (`api.anthropic.com/api/oauth/usage`) and Codex (`chatgpt.com/backend-api/wham/usage`); **surfaced only in Office** agent/dashboard cards | `apps/backend/internal/agent/usage/*`, `internal/backendapp/usage_adapter.go`, consumer `internal/office/agents/service.go` |
| In-band quota from the ACP stream | **Missing** — Claude emits `_claude/rateLimit` (`unifiedWindows`, `resetsAt`, `isUsingOverage`) on every turn; nothing in `apps/` reads it | would live beside `apps/backend/internal/agentctl/server/adapter/` normalization |
| Per-session tokens/cost | Implemented (`task_sessions.tokens_in/out`, `cost_subcents`) — cost is the SDK's API-equivalent estimate, misleading for subscription users | `apps/backend/internal/task/…`, `docs/specs/task-cost-ledger` |
| Account-wide provider usage UI | Plugin (`kdlbs/kandev-plugin-provider-usage`), not built in | `docs/public/agents-and-profiles.md` |
| Provider error classification (quota, rate limit, auth, outage) | Implemented, shared by dynamic profiles and Office | `apps/backend/internal/agent/runtime/routingerr/` (`rules.go`, `provider_neutral_rules.go`, `classify`) |
| Reactive fallback on provider error | **Flagged**: dynamic profiles (`KANDEV_FEATURES_DYNAMIC_AGENT_ROUTING`, off in all profiles) | `apps/backend/internal/agent/runtime/dynamic/` (engine, circuit, conductor), `runtime/dynamic_resolver.go`, `docs/decisions/2026-08-13-dynamic-agent-profile-routing.md` |
| Office provider routing (Frontier/Balanced/Economy tiers, provider order, degrade + backoff, reset hints) | **Flagged** (Office; on in dev profile) | `apps/backend/internal/office/routing/` (`types.go`, `resolver.go`, `backoff.go`), `internal/office/scheduler/dispatch_routing.go`, `routing_lifecycle.go`, `docs/public/office-provider-routing.md` |
| Proactive quota-aware scheduling (route *before* hitting limits, spread load by remaining quota) | **Missing** — ADR says "Subscription and cost routing are a separate future feature" | — |

Extension points for quota-aware routing, in order of leverage:

1. Parse `_claude/rateLimit` (and Codex/OpenCode equivalents if exposed) into a per-profile health record → feed `office/routing` health index (`Resolver.loadHealthIndex`) and `dynamic` circuit instead of only reacting to errors.
2. Generalize `usage.ProviderUsageClient` beyond Anthropic/OpenAI (Gemini, OpenCode Go) and expose it outside Office (profile selector, task top bar).
3. Give `BillingType()` real implementations for OpenCode (per-model provider) and Gemini (OAuth vs API key).

## Office mode

Flagged (`KANDEV_FEATURES_OFFICE`, off in prod, on in dev). Code is large and actively developed (`apps/backend/internal/office/`:
agents, approvals, routines, routing, scheduler, skills, costs, budgets, config sync, …; spec set under `docs/specs/office/`).
It models persistent agent identities (role, instructions, skills, budget, task ownership) separate from execution profiles, with
tier-based provider routing and automatic fallback. Upstream explicitly lists it as "In progress" and "may change between releases".
Not exercised in this bootstrap beyond reading code/docs; the regular workflow engine covered the multi-provider need.

## Weak spots / candidates for fork work

- `mock-agent` not built by `make dev` → noisy Error card in dev.
- Onboarding wizard text says default profiles are auto-approve ("YOLO"); they are not.
- OpenCode/Gemini `billing_type: api_key` label is wrong for subscription users.
- Dev and prod bind `0.0.0.0` with auth off by default.
- Kandev may refresh and rewrite `~/.codex/auth.json` (`usage/client_codex.go`) — shared refresh token with the Codex CLI.
- Agents inherit the parent process environment wholesale; launching Kandev from inside Claude Code leaks `CLAUDECODE`/`ANTHROPIC_BASE_URL`.
- Worktree executor silently branches from `HEAD`; uncommitted work in the source checkout is not visible to the task (by design, but easy to miss — a warning when the source tree is dirty would help).
- Headful/GUI work from agents needs two things: `DISPLAY`/`XAUTHORITY` in the **backend** environment (agents inherit it),
  and a provider sandbox that allows the X socket. Codex's default Kandev mode `agent` ("Approve for me") sandboxes commands
  and blocks `:0`; only `agent-full-access` works, which removes the sandbox entirely. There is no per-profile "allow X11" knob.
- Gemini CLI `gateway` auth type (shared `~/.gemini` with Antigravity) is not usable headless; Kandev's probe still reports `capability_status: ok`.

## Fork divergence log

| Date | Change | Files |
|---|---|---|
| 2026-09-22 | Fork docs + AGENTS.md fork preamble (no code changes) | `AGENTS.md`, `docs/{local-bootstrap,provider-setup,fork-evaluation,upstream-strategy}.md` |
| 2026-09-22 | Task-document MCP tools in task mode (optional `task_id`), task-scoped document HTTP routes + Kanban documents panel, `new_revision` on `update_task_plan_kandev`; Role Pipeline gets a `spike` document slot, a Plan Approval gate, and a conditional escalation return | `apps/backend/internal/mcp/{server,handlers}/`, `apps/backend/internal/backendapp/{helpers,task_document_routes}.go`, `apps/web/components/task/`, `apps/web/lib/api/domains/office-extended-api.ts`, `docs/examples/role-pipeline.workflow.yml`, `docs/specs/tasks/*/kanban-task-documents.md` |

## Run log — Mineflayer Role Pipeline (2026-09-22)

Task `01bdabd9…` in workspace `mineflayer`, Local executor on the real (intentionally dirty) checkout.

| Step | Profile | Result |
|---|---|---|
| Spike | OpenCode `opencode-go/deepseek-v4.1-flash` | Loaded the user's `cityscape-extract-resume-handoff` skill, read Codex session `codex:01a0ca4c…` via the `agents` CLI, verified repo facts and focused tests, saved a 20 KB resume doc as the task plan, called `step_complete_kandev`. No files changed. |
| Architect | Codex `gpt-5.6-sol` | Verified the handoff against the repo, replaced the plan with a 26 KB ordered implementation plan (assumptions audit, gates, rollback), passed a handoff note via `step_complete_kandev` ("Context from the previous workflow step"). No files changed. |
| Implement | Codex `gpt-5.6-terra` | Implemented the aquatic dry-land latch and UUID-exact stale-drop invalidation plus focused regressions; focused tests + `:mod:compileKotlin` pass. Headful gates blocked by the Codex sandbox (see weak spots); user then switched Codex profiles to full access. Paused awaiting the user. |
| Review | Claude `opus[1m]` | Not reached yet. |

Operational lessons:

- Restarting KanDev interrupts running agents; sessions come back `WAITING_FOR_INPUT` on the same step and resume with a message.
- Opening a task re-attaches (not re-prompts) parked sessions (`STARTING → WAITING_FOR_INPUT`, only a `script_execution` row).
- An agent edited KanDev's SQLite directly to flip `auto_approve`; profile **mode** is what actually controls Codex's sandbox
  (`read-only` / `agent` = auto-review sandbox / `agent-full-access`). A running session keeps its launch mode until changed via
  `POST /api/v1/task-sessions/:id/set-config-option {"config_id":"mode",...}` or the composer mode selector.
- KanDev's `session_mode` metadata can disagree with the live ACP `mode` option; trust `acp_model_state.config_options`.
