---
status: draft
system: ui
requirements:
  - REQ-UI-SESSION-TAB-DELETE-FEEDBACK-001
---

# Session Tab Delete Feedback System Design

## Purpose and boundaries

The UI system owns the confirmation and feedback contract for agent-session
deletion. Deletion itself is owned by the task system through
`session.delete`. Closing a session tab no longer deletes anything; close
semantics belong to [Session tab close](session-tab-close.md).

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-UI-SESSION-TAB-DELETE-FEEDBACK-001` | [Confirmation surfaces](#confirmation-surfaces), [Deletion flow](#deletion-flow), [Failure and recovery](#failure-and-recovery) |

## Components and responsibilities

- `DeleteSessionPopover` (`apps/web/components/task/session-tab-menu.tsx`)
  renders the anchored, non-modal confirmation for the desktop context menu
  through the shared `ActionConfirmPopover`.
- `DeleteSessionDialog` in the same module remains for surfaces that need a
  modal confirmation.
- `SessionDeleteDescription` (`session-delete-description.tsx`) owns the shared
  conversation-deletion, workspace-retention, primary-session, and
  only-session warnings.
- `mobile-session-delete-confirmation.tsx` owns the phone Sessions picker
  confirmation step inside the same picker.
- `useSessionActions.remove`
  (`apps/web/hooks/domains/session/use-session-actions.ts`) owns the delete
  request and local-state removal.

## Confirmation surfaces

The desktop context menu keeps its menu mounted and opens
`DeleteSessionPopover` anchored to the Delete item. Dismissing it changes no
state. The phone picker replaces its body with a confirmation step instead of
opening a second modal. Both surfaces render `SessionDeleteDescription`, so the
warnings match.

The `feedback` option passed to `remove`: the menu and mobile paths use the
default `toast` feedback, so progress, success, and error remain visible.

## Deletion flow

1. The user activates Delete from the context menu or the phone picker.
2. The confirmation surface opens without sending a request.
3. On confirm, `removeSessionPanel` is not used; the delete request runs through
   `useSessionActions.remove`.
4. While the request is pending the surface is non-interactive, so a second
   activation does not dispatch another request.
5. On success the session row and its Dockview panel are removed and another
   session becomes active when needed.
6. On failure the local session state and panel remain.

## Failure and recovery

A rejected or timed-out delete request leaves local state and the panel
unchanged, makes the delete action available again, and surfaces one error
toast. A failed primary-session promotion leaves the existing primary session
unchanged and shows one error toast; a successful promotion shows none.

## Responsive behavior

The desktop surface is a viewport-contained anchored popover. The phone surface
keeps the picker's geometry and clears the confirmation when the picker closes
or its context changes. Touch targets stay at least 44 CSS pixels and the
popover does not cause horizontal document overflow.

## Verification

- Component tests assert the X closes without a delete request and that the
  context menu opens the anchored popover without deleting.
- Phone tests assert Cancel restores the picker and external close clears the
  pending confirmation.
- Tests assert a pending delete does not dispatch a second request and a failed
  delete keeps the session and panel.
