# Provider Setup (fork)

How the local subscription-backed agent CLIs were connected to KanDev on 2026-09-22 (upstream `4f76997c8`).

**Rule:** KanDev reuses each CLI's own authenticated state under the OS user running the backend. No OAuth token, API key,
or auth file is copied into this repo or into KanDev profiles. Do not set `ANTHROPIC_API_KEY` / `OPENAI_API_KEY` /
`GEMINI_API_KEY` in the backend environment unless you *want* API billing: an API key in the environment takes precedence
over subscription login for Claude and Codex.

## Summary

| Provider | KanDev agent | Structured mode (default) | Passthrough cmd | Auth reused | Billing observed | Status |
|---|---|---|---|---|---|---|
| Claude Code | `claude-acp` | ACP via managed `npx @agentclientprotocol/claude-agent-acp` (Claude Agent SDK, spawns bundled `claude`) | `npx -y @anthropic-ai/claude-code` | `~/.claude/.credentials.json` (claude.ai OAuth, `claude auth login`) | **Subscription** (verified, see below) | Working |
| Codex | `codex-acp` | ACP via managed `npx @agentclientprotocol/codex-acp` | `npx -y @openai/codex` | `~/.codex/auth.json` (ChatGPT login) | **Subscription** (`billing_type: subscription`) | Working |
| OpenCode | `opencode-acp` | ACP via `opencode acp` | `opencode` | `~/.local/share/opencode/auth.json` (OpenCode Go key + OpenAI OAuth) | Per model's provider: `opencode-go/*` → OpenCode Go subscription; `openai/*` → ChatGPT OAuth; `opencode/*` → OpenCode Zen | Working |
| Gemini CLI | `gemini` | ACP via managed `npx @google/gemini-cli --acp` | `npx --yes --prefer-offline @google/gemini-cli` | `~/.gemini/oauth_creds.json` (Login with Google) | n/a | **Blocked**: see below |

Discovery: `curl localhost:38429/api/v1/agents/discovery` — all four report `available: true` with their host paths.
Source: `apps/backend/internal/agent/agents/{claude_acp,codex_acp,opencode_acp,gemini}.go`.

## Claude Code

1. `claude auth login` (as the backend's OS user, **outside** any Claude Code session env; see `local-bootstrap.md`).
   Before login, `claude auth status` reported `loggedIn: false` even though `~/.claude/.credentials.json` existed (empty/expired token).
2. Default profile "Opus 5" (`opus[1m]`) was auto-created by discovery. Available models from the ACP probe:
   `default`, `opus[1m]`, `claude-fable-5-1[1m]`, `sonnet`, `haiku`.

**ACP vs passthrough and billing.** Both paths run the real Claude Code binary against the same `~/.claude` login, so both
consume subscription quota when no `ANTHROPIC_API_KEY` is present. Evidence from the ACP frame log
(`.kandev-dev/logs/acp/raw-acp-claude-acp-*.jsonl`) of the smoke task:

```json
"_claude/rateLimit": {"isUsingOverage": false, "overageStatus": "rejected", "rateLimitType": "five_hour",
  "status": "allowed", "unifiedWindows": {"five_hour": {"utilization": 0.09}, "seven_day": {"utilization": 0.48}}}
```

`unifiedWindows` + `isUsingOverage:false` is the claude.ai subscription limiter. The `usage_update.cost` field
(`{"amount":0.44,"currency":"USD"}`) is the SDK's *API-equivalent estimate*, not a charge.
KanDev's own detector (`apps/backend/internal/agent/agents/billing.go` → `usage.ClaudeUsageClient.HasSubscriptionCredentials`)
flipped the profile's `billing_type` from `api_key` to `subscription` right after login.

Side effect: ACP Claude loads the user's global `~/.claude` config (plugins, hooks, skills, `CLAUDE.md`) — the smoke task's
agent reported a message from a user-installed plugin. Use a dedicated profile env (`CLAUDE_CONFIG_DIR`) if isolation matters.

Smoke task (sandbox repo, Worktree executor): inspected repo, edited `calc.py`, wrote `NOTES-claude.md`; two permission prompts
surfaced and were approved in the UI; task auto-moved to **Review**; Changes panel showed both files.

## Codex

Already logged in (`codex login status` → "Logged in using ChatGPT"). Default profile "5.6 Sol". Models advertised:
`gpt-6-astra`, `gpt-5.6-sol`, `gpt-5.6-terra`, `gpt-5.6-luna`, `gpt-5.5`. Profiles report `billing_type: subscription`
(`codexBillingType()` reads `~/.codex/auth.json` for ChatGPT OAuth tokens).

Note: KanDev's Codex usage client (`apps/backend/internal/agent/usage/client_codex.go`) can **refresh and rewrite
`~/.codex/auth.json`** when polling usage (Office quota cards). That shares the refresh token with the Codex CLI itself.

## OpenCode

`opencode auth list` → `OpenCode Go (api)` and `OpenAI (oauth)`. Discovery created "OpenCode Zen/Big Pickle"
(`opencode/big-pickle`). For the cheap role we use `opencode-go/deepseek-v4.1-flash` (OpenCode Go subscription).
KanDev labels every OpenCode profile `billing_type: api_key` because `OpenCodeACP.BillingType()` uses the default;
that label is **not** evidence of per-token billing — the real billing is whatever provider the model id routes to.

## Gemini CLI

Installed `@google/gemini-cli` 0.60.0 globally so discovery finds `gemini`. `~/.gemini/settings.json` has
`security.auth.selectedType: "gateway"` (pre-existing; `~/.gemini` is shared with Antigravity) and there is no
`~/.gemini/oauth_creds.json`. Non-interactive `gemini -p ...` fails with "Invalid auth method selected", so KanDev ACP
sessions would fail the same way. To fix (user action): run `gemini`, `/auth` → **Login with Google** (creates
`oauth_creds.json`, switches `selectedType` to `oauth-personal`), then **Settings > Agents > Gemini > refresh**.
KanDev reports Gemini `billing_type: api_key` by default regardless of auth mode.

## Role profiles created for multi-provider work

| Profile | Agent | Model | Role |
|---|---|---|---|
| Spike · DeepSeek v4.1 Flash | OpenCode | `opencode-go/deepseek-v4.1-flash` | cheap context extraction / spikes |
| Architect · GPT-5.6 Sol | Codex | `gpt-5.6-sol` | planning |
| Implementer · GPT-5.6 Terra | Codex | `gpt-5.6-terra` | implementation |
| Reviewer · Opus 5 | Claude | `opus[1m]` | independent review |

Created via `POST /api/v1/agents/:agentId/profiles` (needs the interlock header, see `local-bootstrap.md`).
All have auto-approve **off**. The onboarding dialog claims default profiles run with auto-approve on; in practice they did not.
