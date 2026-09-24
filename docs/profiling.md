# KanDev Desktop Profiling Guide

This guide details the profiling workflow for KanDev Desktop, with a specific focus on diagnosing performance issues on the **Task page**.

KanDev Desktop runs as a multi-layered application consisting of:
1. **Frontend / WebView**: React 19 SPA, Zustand store, Dockview layout, Monaco editor, and xterm terminal streams.
2. **Native Shell**: Tauri v2 desktop process (`kandev-desktop`), WebKit2GTK webview, and OS process management.
3. **Backend & Agent Runtime**: Embedded Go backend (`kandev __backend`), SQLite persistence, and agentctl ACP subprocesses.

The profiling workflow is designed to be **minimal, iteration-friendly, and non-invasive**: it prepares an optimized profiling build with native symbols, exposes loopback-only instrumentation, and allows manual interaction with the running application.

---

## 1. Quickstart Workflow

A standard profiling session follows these 8 steps:

```bash
# Step 1: Launch KanDev Desktop in profiling mode
karto run profiling/desktop

# (Alternatively, focus on task page interactions with the alias)
karto run profiling/task-page
```

1. **Launch:** Run `karto run profiling/desktop`. Kartographer verifies staged runtime binaries, compiles the Tauri shell in optimized release mode with debug symbols and DevTools enabled (`CARGO_PROFILE_RELEASE_DEBUG=true`), creates a timestamped session in `.profile/<session-id>/`, and starts KanDev Desktop with `KANDEV_PROFILE=1`.
2. **Wait for Open:** KanDev Desktop will open and automatically record its process IDs (Tauri shell, Go backend, and child processes) and Go `pprof` endpoint in `.profile/<session-id>/manifest.json`.
3. **Open WebView DevTools:** Press **`F12`** or select **View → Toggle Developer Tools** (or right-click anywhere and select **Inspect Element**).
4. **Activate Desired Profilers:**
   - To profile frontend rendering: Open the **Performance** tab in DevTools and click Record.
   - To profile backend CPU or memory: In a second terminal, run `karto run profiling/backend-cpu` or `karto run profiling/backend-heap`.
   - To sample system-wide native CPU: Launch with `--perf` (`karto run profiling/desktop --perf`).
5. **Manually Exercise the Task Page:** Interact with the application to reproduce the slow behavior (see [Suggested Manual Reproduction Steps](#4-suggested-manual-reproduction-steps-for-task-page) below).
6. **Stop & Export Recordings:**
   - Stop DevTools Performance recording and click **Save Profile** (`.json`). Save the trace file directly into `.profile/<session-id>/`.
   - Backend `.pprof` files are saved automatically into `.profile/<session-id>/`.
7. **Close KanDev:** Close the application window or press `Ctrl+C` in the Kartographer terminal.
8. **Inspect Artifacts & Report:** Kartographer summarizes the session upon exit. You can also view or regenerate the summary at any time:
   ```bash
   karto run profiling/report
   ```

---

## 2. Choosing the Right Profiler

Do not attempt to enable all profilers simultaneously. Select the profiler tailored to your specific hypothesis:

| Layer | Question / Hypothesis | Tool | Primary Metrics / Outputs |
|---|---|---|---|
| **Frontend / UI** | Why is typing laggy? What is causing frame drops during scrolling? Which component re-rendered? | **WebView DevTools (Performance & React)** | Scripting time, style recalculations, layout thrashing, component render durations, JS heap allocations. |
| **Backend (Go)** | Why is the API slow? Where is CPU spent during WebSocket streaming? What is allocating memory? | **Go `pprof`** (`profiling/backend-cpu`, `profiling/backend-heap`) | CPU execution samples by Go function, in-use heap objects, cumulative memory allocations, goroutine count. |
| **Native / System** | Is CPU spent in WebKit rendering, kernel scheduling, or IPC between Tauri and backend? | **Linux `perf`** (`--perf`) / FlameGraph | Hardware CPU cycles, system calls, native shared libraries (`libwebkit2gtk`, `libc`, `libpthread`). |

---

## 3. Profiling Layers & Commands

### A. Frontend Layer: WebView Developer Tools & Chrome DevTools

The profiling build explicitly compiles with Tauri's `devtools` feature enabled, and the backend serves the application on `http://127.0.0.1:38430/`:

#### Option 1: Native WebKit Inspector (Inside Desktop Window)
- **Shortcut:** Press **`F12`** or use **View → Toggle Developer Tools** in the top menu.
- **Inspect Element:** Right-click any UI element and select **Inspect Element**.
- **Timelines Tab (Recording WebKit Timeline):**
  1. Open DevTools → click the **Timelines** tab.
  2. Click **Record** (circle icon in top-left).
  3. Perform the slow user action on the Task Page (scroll messages, stream tokens, switch tasks).
  4. Click **Stop**.
  5. Click the **Export** icon (downward arrow in the toolbar) to export the timeline as a `.json` file.
  6. Save the file into `.profile/latest/` (e.g. `.profile/latest/webkit-timeline.json`).

#### Option 2: External Chrome / Chromium Browser (Recommended for React & Deep Tracing)
Since KanDev Desktop runs a local backend server on `http://127.0.0.1:38430/`, you can open that exact URL in Google Chrome, Chromium, or Brave:
- **Chrome Performance Panel:**
  1. Open `http://127.0.0.1:38430/` in Chrome and press `F12`.
  2. Go to the **Performance** tab and click **Record** (Ctrl+E).
  3. Exercise the Task page.
  4. Click **Stop**, then click the **Save profile** button (downward arrow) to export the trace.
  5. Save to `.profile/latest/chrome-performance.json`.
- **React Developer Tools Profiler:**
  1. In Chrome, switch to the **Profiler** tab (from React DevTools extension).
  2. Check "Record why each component rendered" in Profiler settings.
  3. Click **Start Profiling**, exercise the Task page, and click **Stop Profiling**.
  4. Click the **Export Profiling Data** icon and save to `.profile/latest/react-profile.json`.
  5. This reveals exactly which components (`MessageList`, `TaskPanel`, store subscriber hooks) re-rendered needlessly.

Any `.json` trace file saved in `.profile/latest/` is automatically detected and parsed by `karto run profiling/report`.

### B. Backend Layer: Go `pprof`

When `KANDEV_PROFILE=1` is active, the embedded Go backend starts a dedicated loopback HTTP server on an ephemeral port (e.g. `http://127.0.0.1:40367/debug/pprof/`).

The active endpoint is stored in `.profile/latest/backend-pprof.endpoint`.

#### On-Demand Kartographer Helpers
While KanDev Desktop is running:
```bash
# Capture a 30-second backend CPU profile:
karto run profiling/backend-cpu

# Capture a custom duration (e.g. 15 seconds):
karto run profiling/backend-cpu --seconds 15

# Capture a backend heap memory profile:
karto run profiling/backend-heap
```

#### Inspecting `.pprof` Artifacts
```bash
# Interactive web interface (flamegraph, top functions, source annotation):
go tool pprof -http=127.0.0.1:8080 .profile/latest/backend-cpu-<timestamp>.pprof

# Command-line top CPU consumers:
go tool pprof -top .profile/latest/backend-cpu-<timestamp>.pprof

# Analyze memory allocations:
go tool pprof -http=127.0.0.1:8080 .profile/latest/backend-heap-<timestamp>.pprof
```

### C. Native / System Layer: Linux `perf`

To record whole-process CPU attribution across the Tauri desktop shell, WebKit WebProcess, Go backend, and child processes:

```bash
# Ensure perf permissions are enabled (temporary):
sudo sysctl -w kernel.perf_event_paranoid=1

# Launch KanDev Desktop under perf record:
karto run profiling/desktop --perf
```

If `perf` is not installed or permissions are restricted, KanDev prints a helpful note and proceeds with standard execution without failing.

#### Inspecting `perf.data`
```bash
# Interactive terminal TUI report:
perf report -i .profile/latest/perf.data

# Generate a visual FlameGraph (if FlameGraph scripts or cargo-flamegraph are installed):
perf script -i .profile/latest/perf.data | stackcollapse-perf.pl | flamegraph.pl > .profile/latest/perf-flamegraph.svg
```

---

## 4. Suggested Manual Reproduction Steps for Task Page

Based on codebase analysis and prior performance investigations, the following manual interactions on the Task page are the most valuable to profile:

### 1. Opening a Task with a Long Conversation
- **Action:** In the Kanban board, click to open a task with a long conversation history (e.g. 100+ turns, or the 1,100-message profiling task from `.kandev`).
- **What to Observe:**
  - In DevTools Performance: Main-thread blocking time during initial render, layout recalculations, and markdown parsing time for multiple chat bubbles.
  - In Memory panel: JS heap jump and DOM node count before and after opening.

### 2. Rapid Task Switching
- **Action:** Open task A, then quickly click task B, then task C in the sidebar or back on the Kanban board.
- **What to Observe:**
  - Dockview portal lifecycle: check whether unmounting and remounting editor portals drops frames or leaves orphan DOM trees.
  - State hydration: observe `fetchSessionDataForTask` network timing and `StateHydrator` store updates.

### 3. Viewing Git Changes & Monaco Diffs
- **Action:** Switch to the **Changes** tab on a task with modified files and select a file to open the Monaco Diff Viewer.
- **What to Observe:**
  - Monaco editor worker initialization latency.
  - CPU usage during large diff computation and syntax tokenization.

### 4. Streaming Agent Output
- **Action:** Send a prompt to an agent or run a task turn that produces streaming output.
- **What to Observe:**
  - In DevTools Performance: Frame rate while tokens are received over WebSocket.
  - In `profiling/backend-cpu`: Go backend CPU consumption in WebSocket hub broadcast and agentctl bridge.
  - Observe whether `useProcessedMessages` re-evaluates the entire conversation array on every animation frame.

### 5. Multi-Tab Dockview Layout
- **Action:** Split the layout by opening Chat, Changes, Plan, and a Terminal tab simultaneously.
- **What to Observe:**
  - Background tab processing: verify whether hidden dockview tabs continue running reactive selector subscriptions while inactive.

---

## 5. Artifact Bundle & Cleaning

Every profiling run stores all artifacts in `.profile/<session-id>/`:
- `manifest.json`: Self-describing run manifest (Git commit, dirty status, CPU/OS specs, Tauri PID, Go backend PID, child PIDs, endpoint URLs).
- `desktop.log`: Combined stdout/stderr output from the desktop shell and backend.
- `backend-*.pprof`: Captured Go CPU and heap profiles.
- `perf.data`: Raw Linux perf hardware samples (when `--perf` was used).
- `report.md`: Markdown summary generated by `generate-report.sh`.

### Generating a Report
To summarize the latest profile session:
```bash
karto run profiling/report
```

### Cleaning Up
To delete `.profile/` artifacts without touching desktop build outputs or caches:
```bash
karto run profiling/clean
```
