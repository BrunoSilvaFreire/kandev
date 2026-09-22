# Upstream Strategy (fork)

This fork (`BrunoSilvaFreire/kandev`) tracks `kdlbs/kandev`.

## Fork policy

The fork is in an **experimental, heavy-development phase**: it evaluates Kandev as the foundation for a personal
multi-provider agent orchestration environment (Claude Code, Codex, Gemini CLI, OpenCode on subscription auth).

- **Breaking changes are allowed** when they materially improve architecture, provider integration, routing, or developer
  experience. Prefer clean contracts over compatibility with anything that exists only on this fork.
- **Avoid unnecessary divergence.** If upstream already has a clean mechanism (agent registry, profiles, workflow YAML,
  plugins, runtime flags in `profiles.yaml`), use it.
- **Document divergence** in `docs/fork-evaluation.md` ("Fork divergence log") so syncs stay manageable.
- Never commit credentials, OAuth tokens, agent auth files, or `.kandev-dev/`.
- `AGENTS.md` carries only a two-line fork pointer because the harness lint caps it at 300 lines.

## Remotes

```text
origin    git@github.com:BrunoSilvaFreire/kandev.git   (fetch/push)
upstream  https://github.com/kdlbs/kandev              (fetch; push URL set to DISABLED)
```

Set up with:

```bash
gh repo fork kdlbs/kandev --clone
git remote set-url upstream https://github.com/kdlbs/kandev
git remote set-url --push upstream DISABLED   # never push to upstream by accident
```

`main` tracks `origin/main`.

## Bootstrap baseline

| | |
|---|---|
| Upstream commit | `4f76997c8a0a7218dfcc58858201685eda25b4a1` — "fix: resume ACP sessions without history replay (#3859)" |
| Upstream version | `v0.95.1-8-g4f76997c8` |
| Date | 2026-09-22 |

Measure divergence at any time:

```bash
git fetch upstream
git log --oneline upstream/main..main    # fork-only commits
git log --oneline main..upstream/main    # upstream commits not yet merged
```

## Branch model

- `main` = upstream + fork commits. Fork commits stay small and topical so they rebase cleanly.
- Feature work happens on short-lived branches off `main`, merged back into `main`.
- Anything that should go upstream is developed on a branch off `upstream/main` instead, and PR'd to `kdlbs/kandev`.

## Syncing upstream

Prefer rebase while fork divergence is small (docs/config only):

```bash
git fetch upstream
git switch main
git rebase upstream/main          # replay fork commits on top of upstream
make bootstrap                    # toolchain / lockfile may have moved
make -C apps/backend test && (cd apps && pnpm --filter @kandev/web typecheck)
git push --force-with-lease origin main
```

Switch to merge (`git merge upstream/main`) once the fork carries substantial code changes, so conflict resolutions are recorded once instead of on every rebase.

## Keeping divergence manageable

- Record every substantial local divergence in `docs/fork-evaluation.md` (what, why, files touched).
- Do not reformat or rename upstream files you are not otherwise changing.
- Prefer upstream extension points (agent registry, profiles, workflows YAML, plugins, runtime flags in `profiles.yaml`) over patching core code.
- Upstream moves fast (many commits per day); sync at least weekly to keep conflicts small.
