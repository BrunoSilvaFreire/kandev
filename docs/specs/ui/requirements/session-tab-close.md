---
status: active
system: ui
created: 2026-09-29
owners:
  - kandev
---

# Session Tab Close Requirements

## Overview

Closing a session or conversation tab removes that tab from the current view
state. It never changes or deletes the persisted conversation. Deleting the
conversation is a separate, explicitly destructive action in the tab or
conversation context menu.

The UI system owns this interaction contract because it defines how every
conversation surface presents close and delete consistently. Session lifecycle,
task lifecycle, and conversation persistence remain owned by their existing
systems. The tab model is shared so Task sessions and Quick Chat sessions
behave the same way.

## Terminology

- **Session tab:** A task session tab backed by a Dockview `session:<id>` panel,
  or a Quick Chat tab backed by a persisted Quick Chat task and its primary
  session.
- **Close:** Remove the tab from the current view and persisted tab order
  without changing the server task or session.
- **Delete:** Permanently remove the underlying conversation, session, or
  ephemeral task. Delete is destructive and requires confirmation.
- **Close Others:** Close every other eligible tab in the same tab strip.
- **Close Tabs to the Right:** Close every eligible tab after the target tab in
  the strip order.

## Requirements

### REQ-UI-SESSION-TAB-CLOSE-001: Non-destructive close

**Intent:** Users can tidy their tabs without risking conversation loss.

**User story:** As a user, I want to close a tab without deleting its
conversation, so that I can reopen it later.

#### Acceptance criteria

- **AC-UI-SESSION-TAB-CLOSE-001.1:** When the user activates the close control
  on a Task session tab, the system shall remove that tab from the current view
  and persisted tab order without calling session deletion or task deletion.
- **AC-UI-SESSION-TAB-CLOSE-001.2:** When the user activates the close control
  on a Quick Chat tab, the system shall remove that chat from the current tab
  strip and the persisted Quick Chat tab order for the workspace without
  deleting the backing task or session.
- **AC-UI-SESSION-TAB-CLOSE-001.3:** A closed tab shall not reappear after
  navigation, reload, reconnection, or a live Quick Chat reconciliation, unless
  the user opens it again.
- **AC-UI-SESSION-TAB-CLOSE-001.4:** The server restorable conversation list
  shall continue to include a closed conversation until the user deletes it.
- **AC-UI-SESSION-TAB-CLOSE-001.5:** Closing the active tab shall activate an
  adjacent tab when one exists. Closing the last tab shall leave the empty
  surface without deleting any conversation.
- **AC-UI-SESSION-TAB-CLOSE-001.6:** A Task session shall remain reachable
  after close through the existing session reopen menu. A Quick Chat shall
  remain reachable after close through the Quick Chats browse surface.

### REQ-UI-SESSION-TAB-CLOSE-002: Shared tab context menu

**Intent:** Every conversation tab exposes the same predictable actions.

#### Acceptance criteria

- **AC-UI-SESSION-TAB-CLOSE-002.1:** A Task session tab and a Quick Chat tab
  shall each expose a context menu containing Close, Close Others, Close Tabs
  to the Right, and Delete conversation.
- **AC-UI-SESSION-TAB-CLOSE-002.2:** Close Tabs to the Right shall be disabled
  when the target tab is the rightmost tab. Close Others shall be disabled when
  it has no eligible target.
- **AC-UI-SESSION-TAB-CLOSE-002.3:** Close, Close Others, and Close Tabs to the
  Right shall never change server task or session state, including non-session
  panels such as the task chat panel.
- **AC-UI-SESSION-TAB-CLOSE-002.4:** Delete conversation shall remain the only
  destructive item. Activating it shall require an explicit destructive
  confirmation before any deletion request. Cancelling shall leave the
  conversation and its tab unchanged.
- **AC-UI-SESSION-TAB-CLOSE-002.5:** Quick Chat Delete conversation shall use
  the existing shared confirm-then-delete flow and copy. Task session Delete
  conversation shall use the existing session delete confirmation surfaces.
- **AC-UI-SESSION-TAB-CLOSE-002.6:** The context menu shall be reachable with a
  pointer, keyboard, and touch, and shall return focus to its trigger on
  dismissal.

## Out of scope

- Changing session deletion semantics, active-session selection, or Dockview
  reconciliation.
- Changing the Quick Chat or task delete backend endpoints.
- Replacing delete feedback behavior, which
  [Session tab delete feedback](session-tab-delete-feedback.md) still owns.
- New tab-strip layouts or tab ordering controls.
