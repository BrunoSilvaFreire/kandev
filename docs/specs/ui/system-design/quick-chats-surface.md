---
status: draft
system: ui
requirements:
  - REQ-UI-QUICK-CHATS-SURFACE-001
  - REQ-UI-QUICK-CHATS-SURFACE-002
  - REQ-UI-QUICK-CHATS-SURFACE-003
  - REQ-UI-QUICK-CHATS-SURFACE-004
  - REQ-UI-QUICK-CHATS-SURFACE-005
---

# Quick Chats Surface System Design

## Purpose and boundaries

The UI system owns the Quick Chats navigation entry, browse page, detail page,
capability gate, and creation defaults. It reuses the existing task detail and
session infrastructure instead of a second viewer.

Task and session lifecycle, Quick Chat persistence, and the restorable list
endpoint remain owned by the task system. The Quick Chat panel and its state
slice remain the fast path and are unchanged by this design.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-UI-QUICK-CHATS-SURFACE-001` | [Navigation](#navigation) |
| `REQ-UI-QUICK-CHATS-SURFACE-002` | [Browse page](#browse-page) |
| `REQ-UI-QUICK-CHATS-SURFACE-003` | [Detail page](#detail-page), [Surface capabilities](#surface-capabilities) |
| `REQ-UI-QUICK-CHATS-SURFACE-004` | [Creation flow](#creation-flow) |
| `REQ-UI-QUICK-CHATS-SURFACE-005` | [Browse order, dates, titles, and context](#browse-order-dates-titles-and-context) |

## Navigation

`AppSidebarPrimaryNav` (`apps/web/components/app-sidebar/app-sidebar-primary-nav.tsx`)
renders the Quick Chats item between the Usage item and New Task. The
destination is added to `apps/web/lib/navigation/core-destinations.ts` and to
the mobile surface policy (`apps/web/lib/navigation/surface-policy.ts`), so the
mobile nav sheet and the command palette derive it from the same manifest.

The existing Quick Chat button (`components/task/quick-chat-button.tsx`) and
`QuickChatProvider` modal stay mounted.

## Browse page

The `/quick-chats` route renders a list over `listQuickChatSessions`
(`apps/web/lib/api/domains/workspace-api.ts`). The server returns restorable
Quick Chats only, so the browse list and the tab strip are intentionally
different sets: the tab strip reflects the persisted tab order, and the browse
page reflects the restorable conversations.

Rows reuse the task list row and list primitives. Row actions are open (navigate
to the detail route), open in panel (existing launcher), rename, and delete.
Delete reuses the Phase 1 confirmed delete action.

## Browse order, dates, titles, and context

Three defects in the first browse page share one cause: it presented tab-strip
data as conversation data.

- **Dates.** The row read `task_sessions.updated_at`. That column is rewritten
  by state changes, restart recovery, and the reconciliation sweeps, so after a
  restart every open chat received the same timestamp and the list showed
  clusters of identical ages unrelated to when anyone last wrote.
- **Order.** `ListQuickChatSessions` sorts by task creation time ascending. That
  is the tab strip's stable baseline and is correct there, but it put the
  oldest chat first on the browse page.
- **Titles.** A Quick Chat started without a prompt keeps the placeholder task
  title, which the list deliberately omits as a name, so the row fell back to
  the untitled label even when the conversation had content.

`GET /api/v1/workspaces/:id/quick-chats?include=browse` adds a `browse` object
to each item. The parameter keeps the three extra batch reads off the resync
path, which every client calls on reconnect. Without it the response is
unchanged.

| Field | Source | Notes |
| --- | --- | --- |
| `last_activity_at` | `MAX(task_session_messages.updated_at)` for the primary session over conversation rows only | Rows of type `status`, `log`, `progress`, `error`, and `script_execution` are written by session start, restart, and recovery, so they are excluded; a chat started and never used has only such rows. Falls back to the task's `created_at`. |
| `activity_source` | `message` or `created` | Lets the row say the chat has no messages yet. |
| `title_fallback` | First `user` message of the primary session, whitespace-collapsed and cut to 80 characters | Present only when the chat has no name. Never written to the task. |
| `repository_ids` | `task_repositories` rows of the Quick Chat's task | Attachment only. The model records no per-repository interaction, so the badge says linked and nothing stronger. |
| `tasks` | Unarchived tasks whose `parent_id` is the Quick Chat's task, at most five, with a total count | `create_task_kandev` with `parent_id: "self"` is the durable evidence that the chat created the task. |

The task service authorizes the workspace before any of these reads, and child
tasks share that workspace, so a caller never receives a title from a workspace
it cannot read.

The client resolves repository names from the workspace repository store and
drops an ID it cannot resolve, which covers a deleted repository. Sorting is a
pure function in `apps/web/lib/quick-chat/browse.ts`: activity time, then
session ID, with missing times last in both directions. The order choice is
view state on the page and is not persisted. The title precedence is name, then
`title_fallback`, then the untitled label; rename and agent titling continue to
write the task title through their existing paths and are not changed.

## Detail page

`/quick-chats/:taskId` reuses `TaskDetailRoute`
(`apps/web/src/task-detail-route.tsx`) and `KanbanTaskShell`
(`apps/web/app/tasks/[id]/kanban-task-shell.tsx`) with a `surface` value of
`"quick-chat"`. `TaskPageContent` and `TaskPageInner` render the same dockview,
chat, plan, and documents panels. There is no second conversation viewer.

The route reads `?panel=plan|documents` and opens the matching dockview panel
after the layout is ready. This is the fallback destination for CTAs that have
no dockview in the Quick Chat panel.

## Surface capabilities

One helper, `surfaceCapabilities(surface)`, returns which optional chrome is
available. For `"quick-chat"` it disables the workflow stepper, pull-request
panels, task-state actions, and move/handoff actions. `KanbanTaskShell`,
`TaskPageContent`, and the chat message CTAs read this helper instead of
checking the surface directly. Task-only controls are not rendered at all, so
no inert button remains.

## Creation flow

Agent seeding uses `defaultUtilityAgentProfileId` from the user settings store
(`apps/web/lib/ssr/user-settings.ts`) when the profile is selectable in the
active workspace, otherwise the workspace default agent. An explicit selection
in the setup UI overrides the seed.

The setup's initial prompt uses the shared composer input
(`apps/web/components/task/chat/chat-input-area.tsx` and its body primitive).
The start handler keeps the Config Chat template
(`apps/web/components/config-chat/use-config-chat.ts`): derive the title from
the prompt, call `setQuickChatInitialPrompt` for ACP profiles, and pass the
prompt in `startQuickChat` for passthrough profiles.
`QuickChatSessionView` already hands the stored prompt to
`useQuickChatInitialPrompt`, which submits it through the normal composer once
the session is ready. An empty start skips the prompt path.

## Control flow

1. The sidebar item opens `/quick-chats`.
2. A row opens `/quick-chats/:taskId`, which loads the task and session data
   through `fetchSessionDataForTask` and mounts the shared detail shell.
3. A message CTA inside the detail page adds a dockview panel directly.
4. The same CTA inside the Quick Chat panel navigates to the detail page with
   the `panel` query value.
5. Creating a chat seeds the agent, optionally stores a prompt, and starts the
   session through the existing Quick Chat start path.

## Failure and recovery

A browse list read failure shows the existing list error surface and keeps the
page usable. A detail load failure keeps the route's existing loading and error
handling. A prompt submitted after the session becomes blocked is attempted
once through the existing initial-prompt hook and then cleared, matching Config
Chat.

## Responsive behavior

The browse page uses the same responsive list as Tasks. The detail page uses the
existing task detail responsive layout. The navigation entry appears in the
mobile nav sheet. New top-level navigation and list surfaces keep touch targets
of at least 44 CSS pixels and avoid horizontal document overflow.

## Verification

- Unit tests assert `surfaceCapabilities("quick-chat")` disables each Task-only
  control and `surfaceCapabilities("task")` keeps them.
- Navigation tests assert the Quick Chats destination is in the manifest and
  the mobile policy.
- Playwright covers navigation to the browse page, opening the detail page, and
  Open plan from both the panel and the detail page, including a mobile
  variant.

## Related decisions

- [One filter and grouping model for Home views](../../../decisions/2026-09-29-shared-home-view-model.md)
