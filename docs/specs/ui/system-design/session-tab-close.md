---
status: draft
system: ui
requirements:
  - REQ-UI-SESSION-TAB-CLOSE-001
  - REQ-UI-SESSION-TAB-CLOSE-002
---

# Session Tab Close System Design

## Purpose and boundaries

The UI system owns the shared close-and-delete contract for conversation tabs.
It is expressed through the existing tab primitives rather than a new tab model:

- Task sessions are Dockview panels keyed `session:<id>` in
  `apps/web/lib/state/dockview-panel-actions.ts`.
- Quick Chats are entries in the Quick Chat tab strip, ordered and persisted
  per workspace by `apps/web/components/quick-chat/use-quick-chat-tab-order.ts`.

Conversation persistence, task deletion, and session deletion remain owned by
the task system. This design only decides which local state a close changes.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-UI-SESSION-TAB-CLOSE-001` | [Close paths](#close-paths), [Persistence and resync](#persistence-and-resync), [Failure and recovery](#failure-and-recovery) |
| `REQ-UI-SESSION-TAB-CLOSE-002` | [Shared context menu](#shared-context-menu), [Responsive behavior](#responsive-behavior), [Verification](#verification) |

## Components and responsibilities

- `useTabContextActions` (`apps/web/components/task/use-tab-context-actions.ts`)
  owns the ordered close targeting for Dockview tabs and computes the ids to
  close for Close, Close Others, and Close Tabs to the Right through a pure
  `closeTargets(orderedIds, id, mode)` helper.
- `removeSessionPanel` (`apps/web/lib/state/dockview-panel-actions.ts`) removes
  a `session:<id>` panel without any server call.
- `session-tab-menu.tsx` and the session tab context menu render the shared
  items and keep the existing Delete confirmation.
- `SessionReopenMenuItems`
  (`apps/web/components/task/session-reopen-menu.tsx`) is the reopen path for a
  closed task session.
- `handleCloseTab` and the Quick Chat tab context menu
  (`apps/web/components/quick-chat/use-quick-chat-close-actions.ts`,
  `quick-chat-tab-strip.tsx`) own the analogous Quick Chat path.
- `closeQuickChatSession`/`removeQuickChatSession`
  (`apps/web/lib/state/slices/ui/quick-chat-sync.ts`) remove a Quick Chat from
  the local tab strip and tombstone it so reconciliation does not re-add it.
- `QuickChatDeleteDialog`
  (`apps/web/components/quick-chat/quick-chat-delete-dialog.tsx`) remains the
  only Quick Chat destructive confirmation and is now reached only from the
  Delete conversation item.

## Close paths

For Dockview tabs, `closeTargets` returns the ordered ids to close:

- `close`: the target id only.
- `others`: every eligible id except the target.
- `right`: every eligible id after the target in strip order.

Eligible Dockview panels are non-`chat` panels. Session panels close through
`removeSessionPanel`; other panels close through the existing Dockview removal.
The existing Close Others behavior that spares the shared `chat` panel is
preserved, and session panels are no longer spared.

For Quick Chats, close removes the chat from the persisted tab order and calls
`closeQuickChatSession`/`removeQuickChatSession`. No delete request is sent.

The tab X on a session tab routes to close, not delete. The X on a Quick Chat
tab routes to close. Delete is only reachable from the context menu.

## Shared context menu

One menu model provides Close, Close Others, Close Tabs to the Right, and
Delete conversation for both tab types. Close Others and Close Tabs to the
Right are disabled when they have no target. Delete conversation is separated
from the close items and is styled as destructive. It opens the existing
confirmation surface for its tab type; the deletion request only runs after
confirmation.

## Persistence and resync

- Task sessions: a closed session panel is absent from the serialized Dockview
  layout, so a reload does not reopen it. The session row remains in the task,
  and `SessionReopenMenuItems` can open it again. If the existing layout
  serialization re-adds closed sessions, closed state is recorded in that
  serialization rather than a new store.
- Quick Chats: closed chats are removed from the persisted per-workspace tab
  order. `reconcileQuickChatSessions` drops tombstoned chats, so a reconnect
  resync does not re-add a chat that is missing from the persisted tab order.
  Newly created chats are appended when they are created.

## Failure and recovery

A failed Quick Chat close leaves the chat in the tab strip and keeps the local
order unchanged. A failed deletion leaves the tab and conversation and surfaces
the existing error feedback. Close never fails a server request because it does
not send one.

## Responsive behavior

Desktop tab strips use the context menu. The phone Quick Chat top bar and the
mobile Sessions picker keep their existing close affordances and gain the same
menu items where a context menu is available. Touch targets remain at least
44 CSS pixels and the menu stays inside the viewport safe area.

## Verification

- Pure tests for `closeTargets` cover close, others, right, the rightmost tab,
  and the `chat` panel exclusion.
- Component tests assert the X closes without a delete request and that Delete
  conversation is the only item that opens a confirmation.
- Quick Chat state tests assert a close does not remove the server session and
  that reconciliation does not re-add a closed chat.
- Playwright covers close, Close Others, Close Tabs to the Right, and Delete
  from both tab types, asserting the server still lists the conversation after
  close.

## Related decisions

- [One filter and grouping model for Home views](../../../decisions/2026-09-29-shared-home-view-model.md)
