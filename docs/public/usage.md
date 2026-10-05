---
title: "Usage"
description: "Monitor provider subscription quotas, usage history, and estimates from the Usage page."
---

# Usage

The **Usage** page shows the subscription quota state of every provider account
Kandev can see on this host, with history and a projection for each rate-limit
window. Open it from the sidebar item between Inbox and New Task.

The page is read-only and fetches only while it is open. It does not create a
background poller.

## What it shows

- **Live quota windows** for Claude, Codex, and a locally running Antigravity,
  with the plan, the profiles that share the account, a utilization bar, and a
  reset countdown.
- **Status** per account: exhausted, nearing, stale, healthy, unknown, or
  unavailable. A provider's status is the worst of its accounts.
- **History** per window, built from recorded observations. Measured points are
  solid, estimated points are dashed, and limit hits appear as markers.
- **Estimates** when there are enough points: a burn rate, a projected
  exhaustion time or "resets first", a trend, and a confidence. Every estimate
  is labelled estimated.

Each window's metadata renders as chips. Hover over a chip, or focus it with
the keyboard, to see the detail behind it: the exact local reset time, the last
fetch time and where it came from, the burn rate and its confidence, what the
trend means, and the projected exhaustion time compared with the reset time. A
window without enough samples shows a single "not enough data" chip.

## Balances, subscriptions, and credentials

Some accounts also show a **subscription** line (active, canceling, renewal
pending, or inactive) with the period date and, when the provider keeps serving
after the plan limits, a "uses prepaid balance after limits" chip. Below it,
**balance** rows show the prepaid amount left, such as AIP credits or a USD
prepaid balance for OpenCode Go. Balances and subscription state are
informational and never change the utilization windows; an account whose balance
is fully spent shows a 100% "balance" window.

Two providers need a credential that a local file cannot supply:

- **OpenCode Go** reads its three meters, subscription state, and prepaid
  balance through the OpenCode console session cookie.
- **Junie** reads its credits and license state through the Junie CLI backend,
  using `JUNIE_API_KEY` generated at <https://junie.jetbrains.com/cli>.

On an account that needs one, select **Set credential** (or **Update
credential**) on the Usage page, or open the Quota credentials card on the
agent's settings page. The dialog explains how to obtain the value. It is saved
immediately as a Kandev global secret, is never shown again, and takes effect on
the next read. If the provider rejects it later, the account reports expired
credentials instead of missing ones.

## Breakdown tab

The page has two tabs, **Quotas** and **Breakdown**. The active tab is stored in
the URL as `?tab=breakdown`, so a link opens the Breakdown directly and a reload
keeps you there. The range selector applies to both tabs.

The Breakdown tab reads Kandev's own session usage ledger and answers what is
consuming it. It groups by session, task, model, provider, agent, or UTC day,
and each row shows tokens in, out, and cached, cost, event count, the share of
the filtered total, and the last activity. Sort any column, search by task
title, session name, model, agent, or provider, and filter by provider, model,
or agent.

Selecting a row drills down: the row becomes a filter and the view narrows to
the next dimension (provider to model to session, and agent, task, or day to the
sessions behind the bucket). Active filters appear as removable chips with one
action to clear them all.

On a phone the table becomes stacked cards, the filters move into a bottom
sheet behind a **Filters** button, and sorting is a select.

## Antigravity

Antigravity quota is read from the Antigravity language server that the IDE or
CLI starts. Kandev does not sign in separately and does not use a cloud
endpoint. If Antigravity is not running, its account shows "Antigravity is not
running".

## History sources

Kandev sessions are always indexed: the usage Kandev records for its own runs is
Kandev data and needs no opt-in.

Reading files written by other tools is opt-in, per source:

- **Claude Code history** reads token activity from `~/.claude/projects`.
- **Codex history** reads measured quota windows from `~/.codex/sessions`.
- **Antigravity CLI logs** reads limit hits from `~/.gemini/antigravity-cli/log`.

Nothing under those directories is listed or read while a source is off, and the
data stays on this machine. Turning a source off deletes the history imported
from it.

The first time you open the page, or after enabling a source, Kandev indexes
previous conversations. The page shows progress and polls while the run is
active. Indexing is resumable, skips unchanged files, and continues past a line
it cannot parse. Use **Re-index** to run it again.

## What "estimated" means

Providers report a percentage, not a token budget, so Kandev cannot compute an
exact remaining token count. For Claude, when a live snapshot and token history
are both available, the page scales historical token activity into an estimated
percentage line. It is a guide, not provider data, and it is always labelled
estimated with a confidence.
