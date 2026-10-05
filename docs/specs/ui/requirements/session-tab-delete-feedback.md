---
status: active
system: ui
created: 2026-08-05
updated: 2026-09-29
owners:
  - kandev
---
# Session tab delete feedback Requirements

## Overview

Deleting an agent session is a destructive action available from the session
context menu and the phone Sessions picker. Its confirmation must stay beside
the initiating action where the layout permits, and it must not open a second
blocking surface that hides the session context the user is acting on.

The close control on a session tab closes the tab. It does not delete the
session. Close semantics are owned by
[Session tab close](session-tab-close.md).

The UI system owns this confirmation and feedback contract. Session-deletion
semantics and active-session selection remain owned by the task system.

## Requirements

### REQ-UI-SESSION-TAB-DELETE-FEEDBACK-001: Session tab delete feedback

**Intent:** Session deletion uses a confirmation anchored to its initiating
control and does not add noisy global notifications.

#### Acceptance criteria

- **AC-UI-SESSION-TAB-DELETE-FEEDBACK-001.1:** When the user activates the close
  control on an agent-session tab, the system shall close the tab without
  opening the delete confirmation and without deleting the session.
- **AC-UI-SESSION-TAB-DELETE-FEEDBACK-001.2:** Choosing Delete from a desktop
  session context menu keeps that menu mounted and opens a compact, non-modal
  confirmation popover anchored to the Delete item. Cancelling or dismissing
  the popover leaves the session unchanged.
- **AC-UI-SESSION-TAB-DELETE-FEEDBACK-001.3:** On phone, choosing Delete from a
  Sessions picker row opens a focused confirmation step in the same picker,
  retaining the row's geometry. Cancel restores the picker; external context
  change or closing it clears the unsubmitted confirmation without deleting.
  Presentation follows [mobile action confirmations](mobile-action-confirmations.md).
- **AC-UI-SESSION-TAB-DELETE-FEEDBACK-001.4:** Desktop and phone confirmation
  surfaces share the same conversation-deletion, workspace-retention,
  primary-session, and only-session warnings.
- **AC-UI-SESSION-TAB-DELETE-FEEDBACK-001.5:** After the user confirms, the
  deletion surface is non-interactive and exposes a pending state until the
  delete request settles. Repeated activation while deletion is pending does
  not dispatch another delete request.
- **AC-UI-SESSION-TAB-DELETE-FEEDBACK-001.6:** Successful deletion keeps the
  existing behavior: the deleted session and its tab disappear, and another
  session becomes active when needed. The confirmation surface closes.
- **AC-UI-SESSION-TAB-DELETE-FEEDBACK-001.7:** If deletion fails, the session
  and tab remain, the delete action becomes available again, and one error
  toast explains the failure so the user can retry.
- **AC-UI-SESSION-TAB-DELETE-FEEDBACK-001.8:** Promoting a non-primary agent
  session to primary updates the primary marker without a progress or success
  toast. If the promotion fails, one error toast explains the failure.

## Migrated source detail

## Why

Session deletion is available from several surfaces. Confirmation should stay
beside the initiating action where the layout permits, instead of opening a
second blocking surface that hides the session context the user is acting on.

## What

- Clicking the X on an agent-session tab closes the tab without deleting the
  session. Delete is a separate item in the session context menu.
- Choosing Delete from a desktop session context menu keeps that menu mounted
  and opens a compact, non-modal confirmation popover anchored to the Delete
  item. Cancelling or dismissing the popover leaves the session unchanged.
- On phone, choosing Delete opens a confirmation step in the existing Sessions
  picker with full-width actions. Cancel restores the picker. External context
  changes or closing the picker invalidate the unsubmitted request.
- Desktop and phone confirmation surfaces share the same conversation-deletion,
  workspace-retention, primary-session, and only-session warnings.
- Repeated activation while deletion is pending does not dispatch another
  delete request.
- Successful deletion removes the session and its tab; another session becomes
  active when needed.
- If deletion fails, the session and tab remain, the delete action becomes
  available again, and one error toast explains the failure.
- Promoting a non-primary agent session to primary updates the primary marker
  without a progress or success toast. If the promotion fails, one error toast
  explains the failure.
- After local confirmation, context-menu and mobile deletion retain the default
  request progress, success, and error feedback.

## Failure modes

- A rejected or timed-out delete request does not remove local session state or
  its Dockview panel. The delete action becomes available again and the
  existing error detail is surfaced in one toast.
- A failed primary-session promotion leaves the existing primary session
  unchanged and surfaces one error toast; a successful promotion surfaces no
  progress or success toast.
- Repeated activation while deletion is pending does not dispatch another
  delete request.

## Scenarios

- **GIVEN** a task with two deletable agent sessions, **WHEN** the user clicks
  one tab's X, **THEN** that tab closes and neither session is deleted.
- **GIVEN** an X-closed session, **WHEN** the user opens the session reopen
  menu, **THEN** the closed session is listed and can be opened again.
- **GIVEN** a desktop session context menu, **WHEN** the user chooses Delete,
  **THEN** a compact confirmation popover stays anchored to that menu item
  without opening an alert dialog or starting deletion.
- **GIVEN** the user cancels the delete confirmation, **WHEN** the dialog
  closes, **THEN** the tab remains unchanged and no deletion occurs.
- **GIVEN** a delete request is pending, **WHEN** the user attempts to activate
  Delete again, **THEN** no duplicate delete request is dispatched.
- **GIVEN** a delete request fails, **WHEN** the request settles, **THEN** the
  session tab remains, Delete becomes available again, and one error toast is
  shown.
- **GIVEN** a phone viewport, **WHEN** the user chooses Delete from a Sessions
  picker row, **THEN** the existing picker shows a dedicated confirmation step
  without a second modal.
- **GIVEN** a phone picker has pending delete confirmation, **WHEN** the user
  closes the Sessions picker externally, **THEN** the pending confirmation is
  cleared and reopening the picker shows the normal row actions without
  dispatching deletion.
- **GIVEN** a phone viewport, **WHEN** the user confirms deletion from the
  picker confirmation step, **THEN** the selected session is removed and the
  remaining session stays reachable without relying on a desktop tab X.
- **GIVEN** a task with a non-primary agent session, **WHEN** the user chooses
  Set as Primary from an agent-session action menu, **THEN** the selected
  session becomes primary without a progress or success toast.
- **GIVEN** a primary-session promotion request fails, **WHEN** the request
  settles, **THEN** the current primary session remains unchanged and one error
  toast is shown.

## Out of scope

- Removing or redesigning the tab-X close confirmation dialog.
- Changing backend session-deletion semantics, active-session selection, or
  Dockview reconciliation.
- Changing Sessions picker hierarchy or non-delete row actions beyond hosting
  the shared confirmation step.
- Replacing feedback for context-menu, mobile, stop, or resume actions.
- Close, Close Others, and Close Tabs to the Right behavior, which
  [Session tab close](session-tab-close.md) owns.
