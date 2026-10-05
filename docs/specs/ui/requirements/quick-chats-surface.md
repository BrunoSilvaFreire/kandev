---
status: active
system: ui
created: 2026-09-29
owners:
  - kandev
---

# Quick Chats Surface Requirements

## Overview

Quick Chats are lightweight conversation entities backed by an ephemeral task
and one primary session. They are no longer only a transient side panel: a
dedicated entry in the primary navigation opens a browse page, and each chat
opens into a full detail page that reuses the shared task detail and session
infrastructure.

The UI system owns the navigation placement, browse and detail surfaces, the
task-only capability gate, and the creation defaults. Task and session
lifecycle remain owned by the task system. The Quick Chat panel remains one
fast way to interact with the same entity.

## Terminology

- **Quick Chat:** An ephemeral task with one primary session, created through
  the Quick Chat flow.
- **Quick Chat panel:** The existing modal that hosts Quick Chat tabs.
- **Quick Chat browse page:** The `/quick-chats` page listing restorable Quick
  Chats for the active workspace.
- **Quick Chat detail page:** The `/quick-chats/:taskId` page rendering the
  shared task detail surface.
- **Surface capability:** Whether a Task-only control, such as the workflow
  stepper, is meaningful on a given surface.

## Requirements

### REQ-UI-QUICK-CHATS-SURFACE-001: Navigation entry

**Intent:** Quick Chats become a first-class destination without removing the
fast panel.

**User story:** As a user, I want a Quick Chats entry in the sidebar, so that I
can browse my chats without opening the panel first.

#### Acceptance criteria

- **AC-UI-QUICK-CHATS-SURFACE-001.1:** The desktop primary navigation shall show
  a Quick Chats entry between Usage and New Task in that order.
- **AC-UI-QUICK-CHATS-SURFACE-001.2:** The destination shall be registered in
  the navigation manifest and the mobile navigation equivalent, with the same
  label and destination.
- **AC-UI-QUICK-CHATS-SURFACE-001.3:** The existing Quick Chat button and Quick
  Chat panel shall remain available unchanged.

### REQ-UI-QUICK-CHATS-SURFACE-002: Browse page

**Intent:** Users can find and manage previous Quick Chats.

#### Acceptance criteria

- **AC-UI-QUICK-CHATS-SURFACE-002.1:** Opening `/quick-chats` shall list the
  restorable Quick Chats of the active workspace, each showing its title, its
  agent, and its last activity.
- **AC-UI-QUICK-CHATS-SURFACE-002.2:** Each row shall offer open, open in panel,
  rename, and delete. Delete shall use the same confirmed delete action as the
  tab context menu.
- **AC-UI-QUICK-CHATS-SURFACE-002.3:** The browse page shall reuse the existing
  task-list row and list primitives rather than a second list implementation.
- **AC-UI-QUICK-CHATS-SURFACE-002.4:** An empty result shall show an empty
  state that offers to start a new Quick Chat.
- **AC-UI-QUICK-CHATS-SURFACE-002.5:** Deleting from the browse page shall not
  close or alter unrelated open tabs.

### REQ-UI-QUICK-CHATS-SURFACE-003: Detail page

**Intent:** A substantial Quick Chat opens into a page with the same artifact
capabilities as a Task conversation.

#### Acceptance criteria

- **AC-UI-QUICK-CHATS-SURFACE-003.1:** `/quick-chats/:taskId` shall render the
  shared task detail surface for the backing task, reusing the dockview, chat,
  plan, and documents panels. It shall not introduce a second conversation
  viewer.
- **AC-UI-QUICK-CHATS-SURFACE-003.2:** Task-only chrome and actions shall be
  hidden on a Quick Chat surface: the workflow stepper, pull-request panels,
  task-state actions, and move or handoff actions. They shall not render as
  disabled or inert controls.
- **AC-UI-QUICK-CHATS-SURFACE-003.3:** The capability decision shall come from
  one surface capability helper so every Task-only control is gated the same
  way.
- **AC-UI-QUICK-CHATS-SURFACE-003.4:** Open plan and Open documents CTAs shall
  work on the detail page because it hosts a dockview.
- **AC-UI-QUICK-CHATS-SURFACE-003.5:** Inside the Quick Chat panel, where no
  dockview exists, Open plan and Open documents shall navigate to
  `/quick-chats/:taskId?panel=plan|documents` and open the requested panel.
- **AC-UI-QUICK-CHATS-SURFACE-003.6:** The detail page shall link back to the
  Quick Chat browse page and use Quick Chat terminology, not workflow-step
  terminology.

### REQ-UI-QUICK-CHATS-SURFACE-004: Creation defaults and initial prompt

**Intent:** A new Quick Chat starts with the preferred agent and can be launched
with a first prompt in one action.

#### Acceptance criteria

- **AC-UI-QUICK-CHATS-SURFACE-004.1:** A new Quick Chat shall default its agent
  profile to `defaultUtilityAgentProfileId` when that profile is selectable in
  the workspace. Otherwise it shall fall back to the workspace default agent.
  An explicit user selection shall win over both.
- **AC-UI-QUICK-CHATS-SURFACE-004.2:** The setup UI shall offer an optional
  initial prompt using the shared conversation composer input. It shall not
  introduce a Quick-Chat-only prompt component.
- **AC-UI-QUICK-CHATS-SURFACE-004.3:** When the user provides an initial prompt,
  creating the chat shall create the session, start the selected agent, and
  submit the prompt as the first user message once the session is ready,
  without further user input. The title shall be derived from the prompt as the
  Config Chat flow derives it.
- **AC-UI-QUICK-CHATS-SURFACE-004.4:** Starting a Quick Chat with no prompt shall
  remain supported and shall leave the session idle.
- **AC-UI-QUICK-CHATS-SURFACE-004.5:** The initial-prompt submission shall use
  the existing `setQuickChatInitialPrompt`/`useQuickChatInitialPrompt` path.
  ACP profiles shall store the prompt for the session; passthrough profiles
  shall pass it in the start request.

## Out of scope

- Making a Quick Chat a workflow-confined task or adding workflow steps.
- A per-chat settings surface beyond agent selection and repositories.
- A second conversation renderer for Quick Chats.
- Changing Quick Chat tab ordering, elevation, or viewport layout contracts.
