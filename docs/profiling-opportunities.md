# Task Page Performance Opportunities (Hypotheses for Validation)

> **IMPORTANT**: The items in this document are **hypotheses** identified through source code inspection and review of prior investigation notes in the local SQLite task database (task `9957bf34-6643-4fa8-b76e-9ce93d85a76a`).
> 
> None of these hypotheses should be treated as confirmed facts or implemented as speculative refactors without first collecting empirical evidence using the `karto run profiling/desktop` workflow.

---

## 1. Chat Transcript Virtualization

- **Observation**:
  In `components/task/chat/`, the message history renders every chat bubble and markdown block directly into the DOM tree. In prior database records (`frontend-rendering-cpu/plan.md`), chat virtualization was explicitly marked *out of scope*.
- **Hypothesis**:
  For tasks with long conversations (such as 100+ turns or >1,000 messages), the browser must maintain tens of thousands of DOM elements, recalculate styles across the entire page, and re-layout on each resize or scroll.
- **Evidence to Collect**:
  - In DevTools Performance: Compare layout and style recalculation duration when opening a 10-message task vs. a 500-message task.
  - In DevTools Elements / Memory: Check total DOM node count and JS heap footprint.
- **Potential Solution (to test later)**:
  Use `@tanstack/react-virtual` (which is already a dependency in `apps/web/package.json`) to virtualize the transcript list so only visible bubbles exist in the DOM.

---

## 2. $O(N)$ Streaming Message Processing (`useProcessedMessages`)

- **Observation**:
  During active agent runs, WebSocket messages stream in continuously. Incoming tokens and status updates are batched into the Zustand store on animation frames. The message rendering pipeline (in `useProcessedMessages`) iterates over all conversation turns, normalizing and filtering the entire list.
- **Hypothesis**:
  Re-processing all $N$ messages on every frame during streaming burns significant main-thread CPU time, resulting in dropped frames and typing lag elsewhere in the application shell.
- **Evidence to Collect**:
  - In DevTools Performance: Record a 15-second trace while an agent streams output. Look for recurring long tasks labeled `useProcessedMessages` or store dispatch callbacks.
  - In Go pprof (`profiling/backend-cpu`): Compare backend event dispatch rate against frontend render cadence.
- **Potential Solution (to test later)**:
  Memoize processed message history up to the active streaming turn, updating only the tail item incrementally rather than remapping the entire array.

---

## 3. Zustand Store Subscription Fan-out

- **Observation**:
  There are approximately 485 individual `useAppStore` hook subscriptions across the `components/task/` hierarchy (`TaskPageContent`, `TaskPageInner`, `TaskLayout`, and child tabs).
- **Hypothesis**:
  Even when selectors return primitives or use `shallow` equality, Zustand evaluates all active selector functions whenever *any* part of the store changes (e.g. process status, terminal output, notifications, or task events).
- **Evidence to Collect**:
  - In React DevTools Profiler: Inspect "Why did this render?" and measure commit phase duration during background WebSocket events.
- **Potential Solution (to test later)**:
  Consolidate granular subscriptions, use slice-specific context providers where appropriate, or gate store updates behind scoped event filters.

---

## 4. Dockview Portal Mounts & Monaco Editor Lifecycle

- **Observation**:
  `DockviewDesktopLayout` uses a React portal system (`PanelPortalHost`) to persist certain tab panels across layout rearrangements. When navigating between different tasks, `panelPortalManager.releaseByEnv()` tears down existing portals (including Monaco editor instances and iframes) and reconstructs new ones.
- **Hypothesis**:
  Monaco editor initialization involves web worker communication and heavy DOM insertion. Tearing down and recreating these instances during rapid task navigation creates perceptible UI freezes.
- **Evidence to Collect**:
  - In DevTools Performance: Record a trace during rapid task switching on the Kanban board. Measure the scripting duration between URL navigation and visual stability.
- **Potential Solution (to test later)**:
  Pool or lazily mount Monaco editor instances so they only initialize upon explicitly focusing the Changes or File Editor tab.

---

## 5. Vite `modulepreload` Breadth

- **Observation**:
  In prior task notes, the production HTML build was found to generate 118 `<link rel="modulepreload">` tags for split chunks.
- **Hypothesis**:
  Even when route components and libraries are split with `React.lazy()`, the browser preloads the split chunks immediately upon initial boot, negating network savings and consuming connection pool bandwidth.
- **Evidence to Collect**:
  - In DevTools Network tab: Check waterfall order and concurrency of chunk downloads during initial load.
- **Potential Solution (to test later)**:
  Configure Vite's `build.modulePreload` options or rollup chunking rules to prevent eager preloading of non-critical tab chunks.
