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
- **Backward escalation (Implement → Architect):** no special engine support is needed. The Implement prompt tells the agent
  to call `list_workflow_steps_kandev` + `move_task_kandev(..., entry_options.instructions="ESCALATION: …")` and end its turn;
  the Architect re-enters, revises the plan (plan revisions are kept) or advises, and returns via `move_task_kandev` on the
  default path. With `profile_session_start_policy: reuse` on both steps, KanDev's move preview reports `reuse_other`:
  each role resumes its own parked session instead of starting cold.
- **Conditional approval (superseded 2026-09-23 by the approval card):** the first plan always goes Architect → `Plan Approval` (agent-less human gate) via
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
| Billing-type detection (subscription vs API key) | Implemented for Claude, Codex, and Gemini (when `~/.gemini/oauth_creds.json` carries a token); everything else hard-coded `api_key` | `apps/backend/internal/agent/agents/billing.go`, `BillingType()` on each agent |
| Live quota windows (5h/7d, reset time) via provider usage APIs | Implemented for Claude (`api.anthropic.com/api/oauth/usage`), Codex (`chatgpt.com/backend-api/wham/usage`), Gemini Code Assist (`cloudcode-pa.googleapis.com/v1internal:*`, read-only token), and locally-running Antigravity; surfaced in Office cards and the built-in `/usage` page | `apps/backend/internal/agent/usage/*` (incl. `client_gemini.go`), `internal/providerusage/*`, `internal/backendapp/usage_adapter.go`, consumer `internal/office/agents/service.go` |
| In-band quota from the ACP stream | **Missing** — Claude emits `_claude/rateLimit` (`unifiedWindows`, `resetsAt`, `isUsingOverage`) on every turn; nothing in `apps/` reads it | would live beside `apps/backend/internal/agentctl/server/adapter/` normalization |
| Per-session tokens/cost | Implemented (`task_sessions.tokens_in/out`, `cost_subcents`) — cost is the SDK's API-equivalent estimate, misleading for subscription users | `apps/backend/internal/task/…`, `docs/specs/task-cost-ledger` |
| Account-wide provider usage UI | Built in: top-level `/usage` page with live status, history backfill, and estimates; the plugin (`kdlbs/kandev-plugin-provider-usage`) remains external and untouched | `apps/web/app/usage/`, `apps/backend/internal/providerusage/`, `docs/specs/costs/requirements/provider-usage-monitoring.md` |
| Provider usage history + estimates | Implemented: `task_usage_events` index always on; opt-in Claude/Codex/Antigravity CLI file sources; measured/estimated/limit-hit series | `apps/backend/internal/providerusage/` |
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
- The `agy-acp` bridge always launches with `--no-sandbox --dangerously-skip-permissions` (`AgyACP.ManagedNPMRuntime().ACPArgs`), regardless of the profile's `auto_approve` or mode. The account-suspension risk accepted for this bridge (2026-09-22 row) includes running with permission checks and sandbox off; deriving the flags from profile settings, as other agents do, is not implemented.

## Fork divergence log

| Date | Change | Files |
|---|---|---|
| 2026-09-28 | Relevance-aware workflow session reuse and weighted-random dynamic candidate selection: (1) Part A relevance-aware session reuse excludes exhausted candidates (known quota remaining <= 0), prioritizes the destination step's exit author (`task_step_transitions`), then sessions created for that step, then freshness (`UpdatedAt` tie-break); (2) Part B weighted-random dynamic candidate selection within contiguous known-positive quota bands of the same tag affinity using Efraimidis–Spirakis sampling (`u^(1/w)`), keeping tag affinity dominant, unknown/zero bands deterministic, and legacy deterministic behavior when `Rand` is nil. Rollback: revert orchestrator reuse selector / hint builder and selection ranker. | `apps/backend/internal/{orchestrator,agent/selection,agent/runtime,task/repository/sqlite}`, `docs/specs/{tasks/system-design/workflow-move-preview.md,agents/system-design/dynamic-profile-scheduling.md}`, `docs/fork-evaluation.md` |
| 2026-09-26 | Subscription quota reachability + Gemini Code Assist: (1) fix the usage adapter resolving the agent **type** from the settings agent record (`GetAgent(profile.AgentID).Name`) instead of the profile's `agents.id` UUID, which made every Codex/Claude/Antigravity profile report `no_provider_endpoint` and forced dynamic ranking onto `configured_order`; (2) add a read-only Gemini Code Assist quota client (`~/.gemini/oauth_creds.json`; never refreshed or written; an expired token is unknown, never 0%), a model-scoped `UtilizationWindow.model` so a per-model bucket only affects matching profiles, `ProviderGoogle`/`AgentTypeGemini` identity, and Gemini `BillingType` from the credential file. Rollback: revert the adapter helper and `proxy_usage`/`providerusage` call sites; remove the Gemini client and its adapter/billing cases (no migration). | `apps/backend/internal/{backendapp,providerusage,agent/usage,agent/agents}`, `docs/specs/costs/*`, `docs/decisions/2026-09-26-gemini-code-assist-quota.md`, `docs/provider-setup.md`, `docs/fork-evaluation.md` |
| 2026-09-25 | Refinements to the recently shipped features: `/usage` page wrapped in `PageShell` with human-readable provider `display_name`/account `label`, provider-vs-estimate provenance badges, a closed-set `quota_unavailable_reason`, and UUID-free task-usage rows; cache warmth re-keyed from the newest message to the newest usage event (`last_event_at`); step visits expose their destination sessions across the transition summary, header hover card, and compact disclosure; handoff can optionally move the task to a chosen step via `session.set_primary` then the normal move API; non-plan document selections join the session-scoped comments store as `document-comment`; Enhance with AI gets an explicit context picker (`SelectedContext`, `{{SelectedContext}}`); task-history route rows gain an expandable Details block backed by a nullable `task_session_routes.decision_detail` TEXT column filled by the decision constructors. Additive nullable column plus the existing `runMigrations` path; rollback is reverting the touched files (no destructive migration). | `apps/backend/internal/{providerusage,backendapp,utility,task/{models,dto,service,handlers,repository/sqlite},orchestrator}`, `apps/web/{app/usage,components/usage,components/task,components/task/chat,hooks,lib/state/slices/comments,lib/usage,lib/api,src/locales}`, `apps/web/e2e/tests/{usage,session,task,layout}`, `docs/specs/{costs,task-cost-ledger,tasks,ui}`, `docs/fork-evaluation.md` |
| 2026-09-22 | Fork docs + AGENTS.md fork preamble (no code changes) | `AGENTS.md`, `docs/{local-bootstrap,provider-setup,fork-evaluation,upstream-strategy}.md` |
| 2026-09-22 | Add `agy-acp` third-party bridge for the operator's Antigravity subscription; accepted account-suspension risk. Roll back only this agent, its project MCP strategy, and docs if native or bridge compatibility fails; never redirect profiles to the official kernel. | `apps/backend/internal/agent/{agents,mcpconfig,registry}`, `docs/public/agents-and-profiles.md` |
| 2026-09-22 | Task-document MCP tools in task mode (optional `task_id`), task-scoped document HTTP routes + Kanban documents panel, `new_revision` on `update_task_plan_kandev`; Role Pipeline gets a `spike` document slot, a Plan Approval gate, and a conditional escalation return | `apps/backend/internal/mcp/{server,handlers}/`, `apps/backend/internal/backendapp/{helpers,task_document_routes}.go`, `apps/web/components/task/`, `apps/web/lib/api/domains/office-extended-api.ts`, `docs/examples/role-pipeline.workflow.yml`, `docs/specs/tasks/*/kanban-task-documents.md` |
| 2026-09-22 | Add `karto.kts` pipeline to automate desktop app runtime staging, verification, fast deb packaging, and installation. | `karto.kts`, `docs/{local-bootstrap,fork-evaluation}.md` |
| 2026-09-22 | Tag-based workflow agent selection with quota awareness: free-form `tags` on concrete profiles, `allowed_tags` on workflow steps with OR matching, one quota-ranked selection frozen per entry in the bounded `workflow_session_route`, an Office-independent `POST /api/v1/agent-profiles/utilization` batch endpoint, and settings editors plus a candidate preview. Additive `TEXT NOT NULL DEFAULT '[]'` columns; clearing a step's allowed tags is the rollback. Backend complete; frontend editors/preview added. Quota-driven E2E still needs a mock-usage hook. | `apps/backend/internal/{agent/selection,agent/usage,agent/settings,workflow,task,orchestrator,backendapp}`, `apps/web/{lib,components/settings,hooks/domains/settings}`, `docs/{specs,decisions,plans/tagged-quota-agent-selection,public/agents-and-profiles.md,public/workflow-tips.md}` |
| 2026-09-23 | Resume-with-handoff offer for cache-expired idle sessions: a seeded `builtin-extract-resume-handoff` utility agent (facts-only resume prompt), an offer above the composer when the newest message is older than a 1 h frontend heuristic, and a hook that extracts the handoff, runs the existing `session.reset_context` on the live agent, then sends the handoff as the first prompt of the fresh context. The UI offer is always active when eligible; only its DB row is seeded `enabled=0` (inert, matching `summarize-session`). Live-agent limit: an agentctl `error` entry hides the offer, but a dead agent process on a still-running executor or a `WAITING_FOR_INPUT` session after a backend restart is not detected up front, so `reset_context` rejects with "no active agent execution" after the extraction has run (the context is left untouched; nothing is cached, so a retry re-extracts and re-resets). If the reset succeeds but the send fails, the extracted handoff is placed in the composer instead of being lost. No backend detection or metric; no e2e (an old-message fixture is not available). | `apps/backend/{config/utilityagents/extract-resume-handoff.md,internal/utility/store/{builtins.go,builtins_test.go}}`, `apps/web/{components/task/chat/{resume-handoff-offer.tsx,chat-input-container.tsx,reset-context-button.tsx},hooks/{use-summarize-session.ts,domains/session/use-resume-with-handoff.ts},lib/api/domains/session-reset-api.ts,src/locales}` |
| 2026-09-23 | Session efficiency inspector: the token/context indicator's pinned tooltip becomes a per-session and per-task usage inspector (cache status with estimated expiry, cache hit ratio, token breakdown, cost) with a cache-status badge dot and a warning tint. It reads the existing task-cost-ledger routes (`GET /tasks/:id/usage`, `GET /tasks/:id/sessions/:sid/usage`); no new backend, no second price table. Heuristic thresholds (1 h cache estimate, 0.5 hit ratio after 3 prompts, $5 session cost) are named constants; expiry is labelled an estimate because providers do not expose cache lifetime. Task-chat only (both routes are task-scoped). | `apps/web/{lib/usage/{efficiency.ts,format.ts,newest-message.ts},lib/api/domains/usage-api.ts,hooks/domains/session/use-session-usage-inspector.ts,components/task/chat/{token-usage-display.tsx,token-usage-inspector.tsx,chat-input-toolbar-desktop.tsx,chat-input-toolbar-mobile.tsx}},src/locales` |
| 2026-09-23 | Per-agent Usage panel + `GET /tasks/:id/usage/breakdown`: one read-only grouped route over the existing priced ledger (finest grain session/profile/type/model/provider, no new pricing, no transaction, no migration) and one reusable dockview panel `usage` wired exactly like `prompt-history`, with a mobile "More" entry, a table at >= md and stacked cards below md. By agent / By model / By session are client-side additive roll-ups; the hit ratio is recomputed from the summed tokens. Supersedes the rev 2 "no dockview panel / no backend work" choice for the detailed view; the pinned popup stays the quick view. Sessions with no ledger rows are merged from the store. | `apps/backend/internal/task/{dto,handlers,service,models,repository}`, `apps/web/{lib/usage/breakdown.ts,lib/api/domains/usage-api.ts,hooks/domains/session/use-task-usage-breakdown.ts,components/task/usage-panel,hooks/domains/session,components/task/{dockview-shared.tsx,dockview-panel-content.tsx,dockview-desktop-layout.tsx,dockview-add-panel-items.tsx},components/task/mobile,lib/state/{layout-manager,dockview-store.ts,dockview-extra-panel-actions.ts,slices/ui/types.ts},src/locales}`, `docs/specs/task-cost-ledger/spec.md` |
| 2026-09-23 | Antigravity ACP fixes: fork `shindgew/agy-acp` (`BrunoSilvaFreire/agy-acp`), prefer local native `agy-acp` binary on `$PATH` in `AgyACP` agent. Fix premature turn completions (track step 101 after user input) and bridge tool permissions for non-`ask_question` tools in `agy-acp`. In Kandev ACP normalizer and adapter, accept PascalCase tool arguments (`CommandLine`, `Cwd`, `AbsolutePath`, `TargetFile`, `SearchPath`, `Query`, `IsDaemon`), enrich diffs from initial `tool_call` contents, and avoid tracking pre-completed tool calls as active so `cancelPromptEndToolCalls` does not mark them cancelled. | `apps/backend/internal/agent/agents/agy_acp.go`, `apps/backend/internal/agentctl/server/adapter/transport/acp/{normalize.go,adapter_tools.go}` |
| 2026-09-23 | Role pipeline v2: (1) first-class approval as a clarification kind (`request_approval_kandev` creates one fixed approve/revise/reject question carrying `approval` metadata; the resolver records the subject version at request time, renders pending plan comments pre-claim, reports `subject_edited`/`current_version`, and consumes comments best-effort after delivery); (2) named step transitions stored in the existing `events.transitions` JSON and invoked via `move_task_kandev(transition=...)` (no new column, no engine trigger; direction checked at invocation, one-shot options merged with the caller's); (3) the Role Pipeline example drops the agent-less `Plan Approval` gate and uses the in-chat approval card plus `handoff`/`escalate`/`request_changes` transitions; (4) Enhance Prompt streams NDJSON phase status over the existing HTTP request (no WS). Supersedes the 2026-09-22 Plan Approval gate row's conditional-gate design. Deferred: inbox approve/reject quick actions, plan comments on non-plan documents, an engine-enforced first-plan-approval rule, and a transitions UI/kanban buttons/built-in-workflow transitions. | `apps/backend/internal/{clarification,mcp/{server,handlers},task/service,workflow/{models,controller},sysprompt,backendapp}`, `apps/web/{lib/types/workflow-actions.ts,...}`, `docs/examples/role-pipeline.workflow.yml`, `docs/specs/tasks/{requirements,system-design}/{approval-requests,named-step-transitions}.md`, `docs/fork-evaluation.md` |
| 2026-09-25 | Consolidate schedule-time ranking into Dynamic Profiles: soft `preferred_tags`/`avoided_tags` on `dynamic_agent_profiles`, quota/tag/circuit ranking reusing the existing `internal/agent/selection` comparator and `usage.RemainingPct`, five closed route reasons, and deterministic conversion of legacy `workflow_steps.allowed_tags` into generated Dynamic Profiles. The legacy tagged workflow selector is retained during the compatibility release and removed only after the migration/parity gates pass. Additive `TEXT NOT NULL DEFAULT '[]'` columns; prior binary keeps working. See `docs/decisions/2026-09-25-dynamic-profile-scheduling.md`. | `apps/backend/internal/{agent/runtime/dynamic,agent/runtime,agent/settings,agent/usage,workflow,backendapp}`, `apps/web/components/settings`, `docs/{specs/agents,decisions,plans}` |
| 2026-09-24 | First-class provider usage monitoring: top-level `/usage` page (sidebar item between Inbox and New Task, no Office dependency), `internal/providerusage` observation store + resumable single-flight history index, an always-on `task_usage_events` source plus opt-in local Claude/Codex/Antigravity CLI file sources, a local-loopback Antigravity language-server client, a `UsageCache` fetch recorder, a lifecycle quota/rate-limit signal recorder, estimation/calibration, and `GET/POST /api/v1/provider-usage*`. Rolling back means deleting the `/usage` route, sidebar item, and `internal/providerusage`, and reverting the recorder/lifecycle wiring. | `apps/backend/internal/{providerusage,agent/usage,backendapp,agent/runtime/lifecycle}`, `apps/web/{app/usage,components,lib,hooks,src}`, `docs/specs/costs/*`, `docs/decisions/2026-09-24-local-provider-usage-sources.md` |
| 2026-09-24 | Headless Debian systemd service runtime (`service/*`) and bidirectional safe data transfer (`data/*`) in `karto.kts`. Runs persistent headless KanDev under `systemd --user` with dedicated data directory `~/.kandev-service`, serving backend and Vite SPA at `http://kandev.local/` (port 80). Adds `ServiceSetupHostTask` with non-elevated early-exit check, `ServiceInstallTask` using native `kandev service install --port 80 --home-dir ~/.kandev-service`, service control tasks (`start`, `stop`, `restart`, `status`, `logs`, `uninstall`), and pure Kotlin `DataTransferTask` (`data/desktop-to-service`, `data/service-to-desktop`) with active PID-lock checking, destination timestamped backups, symlink preservation, and service pause/resume. | `karto.kts`, `docs/fork-evaluation.md` |
| 2026-09-26 | Add `junie-acp`, a native-binary ACP agent for JetBrains Junie (`junie --acp=true`): PATH discovery, `JUNIE_API_KEY` remote auth, official `install.sh` card, `{home}/.junie` session root, and a forced new session on workspace rebind because Junie's `session/load` is project-scoped. The Junie-only ACP probe waits up to five seconds for Junie's delayed model advertisement. Also extends the ACP probe allowlist with `junie`. Measured against `junie` 26.6.29 (linux-x86_64); no managed npm runtime, frontend, or Office routing change. | `apps/backend/internal/agent/agents/{junie_acp.go,logos/junie_*.svg,new_acp_agents_test.go,workspace_rebind_resume_test.go}`, `apps/backend/internal/agent/registry/`, `apps/backend/internal/agentctl/{acpcompat/junie.go,server/utility/acp_executor.go}`, `docs/public/agents-and-profiles.md`, `docs/specs/agents/{requirements,system-design}/junie-acp-agent.md`, `docs/plans/junie-agent/` |

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
- Editing a step prompt does not reach a session already in that step: prompts are delivered on step entry only. When asked to
  "escalate to the Architect" without that prompt, Codex spawned its own native `architect` subagent and then called
  `step_complete_kandev`, which advanced the task to Review. Step prompts must name the KanDev mechanism (`move_task_kandev`
  to the step named "Architect") and forbid native subagents for workflow roles.
- KanDev's `session_mode` metadata can disagree with the live ACP `mode` option; trust `acp_model_state.config_options`.
- The `Plan Approval` gate is example-only: editing `docs/examples/role-pipeline.workflow.yml` does not change a workflow already
  imported into a workspace, and a step prompt change never reaches a session already in that step. To adopt the gate, re-import the
  example (Settings > Workspaces > Workflows > Import) into a scratch or refreshed workspace **before** a task reaches Architect, or
  move an in-flight task's card by hand. The gate has no `on_enter` auto-start and no `on_turn_complete` transition by design, so only
  a human move advances past it.
- The ADR 0015 `step_complete_kandev` handoff does **not** cross an agent-less gate step. The carry token
  (`tasks.metadata.step_handoff_carry`) is a single slot keyed to the immediate next step and is claimed only when that step
  dispatches a prompt; the gate never does, and the token's step no longer matches once a human moves the task on. Prompts must not
  promise handoff delivery across the gate: the Architect's authoritative output is the plan, which the Implementer reads with
  `get_task_plan_kandev`. Extending the carry across agent-less steps would be a separate engine change.
  **Superseded (2026-09-23):** the Role Pipeline example no longer has an agent-less gate, so this carry limitation no longer applies
  to it. The Architect now approves in chat through `request_approval_kandev`, and escalation returns use the named `handoff` and
  `escalate` transitions, which carry their instructions as one-shot entry options on the move itself.
