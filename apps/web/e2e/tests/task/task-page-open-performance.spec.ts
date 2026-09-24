import {
  devices,
  type Browser,
  type BrowserContext,
  type Page,
  type TestInfo,
} from "@playwright/test";

import { test, expect, type SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import {
  captureTraceEvents,
  installPerformanceObservers,
  parsePositiveInteger,
  readPerformanceMetrics,
  readPerformanceObservers,
  startNetworkCapture,
  startTrace,
  stopTrace,
  summarizeNumbers,
  summarizeTraceMetric,
  traceMetricNamed,
  type LongTaskSummary,
  type NetworkEntry,
  type TraceEvent,
  type TraceMetric,
} from "../../helpers/cdp-trace";
import { multiMessageScript } from "../../helpers/seed-session-messages";

/**
 * Opt-in task-page performance profile.
 *
 * This spec does not gate anything and is not part of the default suite. It
 * measures the task page across the scenario matrix from the task-page
 * performance spike so a change can be judged on a warmed production build.
 * Run it explicitly:
 *
 * ```sh
 * cd apps/web
 * KANDEV_E2E_TASK_PAGE_TRACE=1 KANDEV_E2E_TASK_PAGE_TRACE_REPEATS=5 \
 *   pnpm e2e:run --no-build --project chromium tests/task/task-page-open-performance.spec.ts
 * ```
 *
 * Artifacts (raw trace + measurement JSON) are attached to the Playwright
 * report, and a machine-readable summary is printed via `console.info` with the
 * `[task-page-open-performance]` prefix.
 */

const TRACE_ENABLED = process.env.KANDEV_E2E_TASK_PAGE_TRACE === "1";
const TRACE_REPEATS = parsePositiveInteger(
  process.env.KANDEV_E2E_TASK_PAGE_TRACE_REPEATS ??
    process.env.KANDEV_E2E_ANIMATION_TRACE_REPEATS ??
    "1",
);
const STEADY_WINDOW_MS = 5_000;

const OPEN_TASK_TIMEOUT = 45_000;
const PROMPT_TEXT = "TASK-PAGE-PERF-PROMPT";
const TEXT_PREFIX = "TASK-PAGE-PERF-TEXT";

test.skip(!TRACE_ENABLED, "Run explicitly with KANDEV_E2E_TASK_PAGE_TRACE=1 to profile.");
test.describe.configure({ retries: 0 });

type TaskSeed = {
  taskId: string;
  sessionId: string;
  /** Text of the newest seeded message; the transcript-ready signal. */
  readyText: string | null;
};

type OpenMeasurement = {
  label: string;
  navToChatMs: number;
  navToRowsMs: number;
  mountedRows: number;
  domNodes: number;
  heapDeltaBytes: number;
  requests: number;
  transferBytes: number;
  networkByUrl: NetworkEntry[];
  longTasks: LongTaskSummary;
  largestContentfulPaintMs: number | null;
  trace: Record<string, TraceMetric>;
};

type SteadyMeasurement = {
  label: string;
  mountedRows: number;
  domNodes: number;
  longTasks: LongTaskSummary;
  trace: Record<string, TraceMetric>;
};

const TRACE_METRIC_NAMES = [
  "RunTask",
  "FunctionCall",
  "Layout",
  "Paint",
  "UpdateLayoutTree",
  "Layerize",
] as const;

function reduceTrace(events: TraceEvent[]): Record<string, TraceMetric> {
  const reduced: Record<string, TraceMetric> = {};
  for (const name of TRACE_METRIC_NAMES) reduced[name] = traceMetricNamed(events, name);
  return reduced;
}

// ---------------------------------------------------------------------------
// Seeding
// ---------------------------------------------------------------------------

async function seedTask(
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
): Promise<{ taskId: string; sessionId: string }> {
  const task = await apiClient.createTask(seedData.workspaceId, title, {
    description: "TASK-PAGE-PERF-MARKER",
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });
  const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, {
    state: "IDLE",
    repositoryId: seedData.repositoryId,
  });
  return { taskId: task.id, sessionId };
}

/** A: minimal task, no extra messages. */
async function seedMinimalTask(
  apiClient: ApiClient,
  seedData: SeedData,
): Promise<TaskSeed> {
  const { taskId, sessionId } = await seedTask(apiClient, seedData, "Perf A minimal");
  return { taskId, sessionId, readyText: null };
}

/** B/H: text-heavy transcript (one prompt + N agent text rows). */
async function seedTextHeavyTask(
  apiClient: ApiClient,
  seedData: SeedData,
  count = 100,
): Promise<TaskSeed> {
  const { taskId, sessionId } = await seedTask(apiClient, seedData, `Perf B text x${count}`);
  await apiClient.seedSessionMessage(sessionId, { type: "message", content: PROMPT_TEXT, authorType: "user" });
  await apiClient.seedAgentMessages(sessionId, count, TEXT_PREFIX);
  return { taskId, sessionId, readyText: `${TEXT_PREFIX} ${count}` };
}

/** C: tool-heavy transcript (collapsed tool groups). */
async function seedToolHeavyTask(
  apiClient: ApiClient,
  seedData: SeedData,
  count = 100,
): Promise<TaskSeed> {
  const { taskId, sessionId } = await seedTask(apiClient, seedData, `Perf C tools x${count}`);
  await apiClient.seedSessionMessage(sessionId, { type: "message", content: PROMPT_TEXT, authorType: "user" });
  await apiClient.seedToolCallMessages(sessionId, count, { status: "complete" });
  return { taskId, sessionId, readyText: PROMPT_TEXT };
}

/** F: large history that must paginate behind the initial 100-message window. */
async function seedLargeHistoryTask(
  apiClient: ApiClient,
  seedData: SeedData,
  count = 500,
): Promise<TaskSeed> {
  const { taskId, sessionId } = await seedTask(apiClient, seedData, `Perf F history x${count}`);
  await apiClient.seedSessionMessage(sessionId, { type: "message", content: PROMPT_TEXT, authorType: "user" });
  await apiClient.seedAgentMessages(sessionId, count, TEXT_PREFIX);
  return { taskId, sessionId, readyText: `${TEXT_PREFIX} ${count}` };
}

/** G: several sessions, each with its own transcript, all opened as panels. */
async function seedMultiSessionTask(
  apiClient: ApiClient,
  seedData: SeedData,
  sessions = 4,
  messagesEach = 100,
): Promise<{ taskId: string; sessionIds: string[]; readyText: string }> {
  const task = await apiClient.createTask(seedData.workspaceId, "Perf G multi-session", {
    description: "TASK-PAGE-PERF-MARKER",
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });
  const sessionIds: string[] = [];
  for (let index = 0; index < sessions; index += 1) {
    const { session_id } = await apiClient.seedTaskSession(task.id, {
      state: "IDLE",
      repositoryId: seedData.repositoryId,
    });
    sessionIds.push(session_id);
    const prefix = `${TEXT_PREFIX}-S${index + 1}`;
    await apiClient.seedSessionMessage(session_id, {
      type: "message",
      content: `${PROMPT_TEXT} S${index + 1}`,
      authorType: "user",
    });
    await apiClient.seedAgentMessages(session_id, messagesEach, prefix);
  }
  return {
    taskId: task.id,
    sessionIds,
    // Each session has one prompt + N agent rows, so the prompt falls outside
    // the newest-100 window. Every agent row shares this prefix, so it is the
    // reliable ready signal whichever session panel is foregrounded.
    readyText: `${TEXT_PREFIX}-S`,
  };
}

// ---------------------------------------------------------------------------
// Measurement
// ---------------------------------------------------------------------------

async function resetObserverBuffers(page: Page): Promise<void> {
  await page.evaluate(() => {
    const observerWindow = window as Window & {
      __kandevLongTasks?: number[];
      __kandevLcp?: number[];
    };
    observerWindow.__kandevLongTasks = [];
    observerWindow.__kandevLcp = [];
  });
}

/**
 * Navigate to the task and measure cold-open latency plus the trace covering
 * the open. The trace runs from before `goto` until the transcript is ready, so
 * it captures exactly the open window rather than a fixed sample.
 */
async function measureOpen(
  page: Page,
  taskId: string,
  readyText: string | null,
  testInfo: TestInfo,
  label: string,
): Promise<OpenMeasurement> {
  const client = await page.context().newCDPSession(page);
  await installPerformanceObservers(page);
  await resetObserverBuffers(page);
  const before = await readPerformanceMetrics(client, { collectGarbage: true });
  const stopNetwork = await startNetworkCapture(client);
  await startTrace(client, { disableScriptExecution: false });

  const start = Date.now();
  await page.goto(`/t/${taskId}`, { waitUntil: "domcontentloaded" });
  await page.getByTestId("session-chat").first().waitFor({
    state: "attached",
    timeout: OPEN_TASK_TIMEOUT,
  });
  const navToChatMs = Date.now() - start;
  await page.locator(".chat-message-list").first().waitFor({
    state: "visible",
    timeout: OPEN_TASK_TIMEOUT,
  });
  if (readyText) {
    // Scope to the visible chat panel: multi-session tasks can retain hidden
    // session panels whose transcripts also contain the shared prompt text.
    const visibleChat = page.locator("[data-testid='session-chat']:visible").first();
    await expect
      .poll(() => visibleChat.getByText(readyText, { exact: false }).count(), {
        timeout: OPEN_TASK_TIMEOUT,
        message: `Waiting for transcript tail "${readyText}"`,
      })
      .toBeGreaterThan(0);
  } else {
    await expect.poll(() => page.locator("[id^='msg-']").count(), { timeout: OPEN_TASK_TIMEOUT }).toBeGreaterThan(0);
  }
  const navToRowsMs = Date.now() - start;

  const mountedRows = await page.locator("[id^='msg-']").count();
  const domNodes = await page.evaluate(() => document.getElementsByTagName("*").length);
  const observers = await readPerformanceObservers(page);
  const after = await readPerformanceMetrics(client, { collectGarbage: true });
  const network = stopNetwork();
  const events = await stopTrace(client, testInfo, label);

  return {
    label,
    navToChatMs,
    navToRowsMs,
    mountedRows,
    domNodes,
    heapDeltaBytes: (after.JSHeapUsedSize ?? 0) - (before.JSHeapUsedSize ?? 0),
    requests: network.requestCount,
    transferBytes: network.encodedDataLength,
    networkByUrl: network.byUrl.slice(0, 25),
    longTasks: observers.longTasks,
    largestContentfulPaintMs: observers.largestContentfulPaintMs,
    trace: reduceTrace(events),
  };
}

/** A fixed-window trace on an already-open page (streaming or multi-tab). */
async function measureSteady(
  page: Page,
  testInfo: TestInfo,
  label: string,
): Promise<SteadyMeasurement> {
  await resetObserverBuffers(page);
  const events = await captureTraceEvents(page, testInfo, label, STEADY_WINDOW_MS, {
    disableScriptExecution: false,
  });
  const observers = await readPerformanceObservers(page);
  return {
    label,
    mountedRows: await page.locator("[id^='msg-']").count(),
    domNodes: await page.evaluate(() => document.getElementsByTagName("*").length),
    longTasks: observers.longTasks,
    trace: reduceTrace(events),
  };
}

// ---------------------------------------------------------------------------
// Reporting
// ---------------------------------------------------------------------------

function summarizeOpens(measurements: OpenMeasurement[]) {
  const series = (read: (measurement: OpenMeasurement) => number) =>
    summarizeNumbers(measurements.map(read));
  return {
    repeats: measurements.length,
    navToChatMs: series((measurement) => measurement.navToChatMs),
    navToRowsMs: series((measurement) => measurement.navToRowsMs),
    mountedRows: series((measurement) => measurement.mountedRows),
    domNodes: series((measurement) => measurement.domNodes),
    heapDeltaBytes: series((measurement) => measurement.heapDeltaBytes),
    requests: series((measurement) => measurement.requests),
    transferBytes: series((measurement) => measurement.transferBytes),
    longTaskCount: series((measurement) => measurement.longTasks.count),
    longTaskMaxMs: series((measurement) => measurement.longTasks.maxMs),
    largestContentfulPaintMs: measurements.some(
      (measurement) => measurement.largestContentfulPaintMs !== null,
    )
      ? summarizeNumbers(
          measurements.map((measurement) => measurement.largestContentfulPaintMs ?? 0),
        )
      : null,
    trace: Object.fromEntries(
      TRACE_METRIC_NAMES.map((name) => [
        name,
        summarizeTraceMetric(measurements.map((measurement) => measurement.trace[name])),
      ]),
    ),
  };
}

async function report(
  testInfo: TestInfo,
  name: string,
  payload: Record<string, unknown>,
): Promise<void> {
  await testInfo.attach(`${name}-report.json`, {
    body: Buffer.from(JSON.stringify(payload, null, 2)),
    contentType: "application/json",
  });
  console.info(`[task-page-open-performance] ${JSON.stringify({ scenario: name, ...payload })}`);
}

// ---------------------------------------------------------------------------
// Scenarios
// ---------------------------------------------------------------------------

async function newDesktopContext(browser: Browser, baseURL: string): Promise<BrowserContext> {
  return browser.newContext({ baseURL });
}

test.describe.serial("task page open performance", () => {
  test.setTimeout(1_800_000);

  test("A cold open, minimal task", async ({ browser, backend, apiClient, seedData }, testInfo) => {
    const seed = await seedMinimalTask(apiClient, seedData);
    const measurements: OpenMeasurement[] = [];
    for (let repeat = 1; repeat <= TRACE_REPEATS; repeat += 1) {
      const context = await newDesktopContext(browser, backend.frontendUrl);
      const page = await context.newPage();
      try {
        measurements.push(
          await measureOpen(page, seed.taskId, seed.readyText, testInfo, `A-${repeat}`),
        );
      } finally {
        await context.close();
      }
    }
    await report(testInfo, "A", { summary: summarizeOpens(measurements), measurements });
  });

  test("B cold open, text-heavy", async ({ browser, backend, apiClient, seedData }, testInfo) => {
    const seed = await seedTextHeavyTask(apiClient, seedData);
    const measurements: OpenMeasurement[] = [];
    for (let repeat = 1; repeat <= TRACE_REPEATS; repeat += 1) {
      const context = await newDesktopContext(browser, backend.frontendUrl);
      const page = await context.newPage();
      try {
        measurements.push(
          await measureOpen(page, seed.taskId, seed.readyText, testInfo, `B-${repeat}`),
        );
      } finally {
        await context.close();
      }
    }
    await report(testInfo, "B", { summary: summarizeOpens(measurements), measurements });
  });

  test("C cold open, tool-heavy", async ({ browser, backend, apiClient, seedData }, testInfo) => {
    const seed = await seedToolHeavyTask(apiClient, seedData);
    const measurements: OpenMeasurement[] = [];
    for (let repeat = 1; repeat <= TRACE_REPEATS; repeat += 1) {
      const context = await newDesktopContext(browser, backend.frontendUrl);
      const page = await context.newPage();
      try {
        measurements.push(
          await measureOpen(page, seed.taskId, seed.readyText, testInfo, `C-${repeat}`),
        );
      } finally {
        await context.close();
      }
    }
    await report(testInfo, "C", { summary: summarizeOpens(measurements), measurements });
  });

  test("D warm reopen", async ({ browser, backend, apiClient, seedData }, testInfo) => {
    const seed = await seedTextHeavyTask(apiClient, seedData);
    const context = await newDesktopContext(browser, backend.frontendUrl);
    const page = await context.newPage();
    const measurements: OpenMeasurement[] = [];
    try {
      for (let repeat = 1; repeat <= TRACE_REPEATS; repeat += 1) {
        measurements.push(
          await measureOpen(page, seed.taskId, seed.readyText, testInfo, `D-${repeat}`),
        );
      }
    } finally {
      await context.close();
    }
    await report(testInfo, "D", { summary: summarizeOpens(measurements), measurements });
  });

  test("E streaming while open", async ({ browser, backend, apiClient, seedData }, testInfo) => {
    const streamLines = Array.from({ length: 600 }, (_, index) => `${TEXT_PREFIX}-STREAM ${index + 1}`);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Perf E streaming",
      seedData.agentProfileId,
      {
        description: multiMessageScript(streamLines, 25),
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    const context = await newDesktopContext(browser, backend.frontendUrl);
    const page = await context.newPage();
    const opens: OpenMeasurement[] = [];
    const steady: SteadyMeasurement[] = [];
    try {
      for (let repeat = 1; repeat <= TRACE_REPEATS; repeat += 1) {
        opens.push(await measureOpen(page, task.id, null, testInfo, `E-open-${repeat}`));
        steady.push(await measureSteady(page, testInfo, `E-steady-${repeat}`));
      }
    } finally {
      await context.close();
    }
    await report(testInfo, "E", {
      summary: summarizeOpens(opens),
      steady,
      measurements: opens,
    });
  });

  test("F large history", async ({ browser, backend, apiClient, seedData }, testInfo) => {
    const seed = await seedLargeHistoryTask(apiClient, seedData);
    const measurements: OpenMeasurement[] = [];
    for (let repeat = 1; repeat <= TRACE_REPEATS; repeat += 1) {
      const context = await newDesktopContext(browser, backend.frontendUrl);
      const page = await context.newPage();
      try {
        measurements.push(
          await measureOpen(page, seed.taskId, seed.readyText, testInfo, `F-${repeat}`),
        );
      } finally {
        await context.close();
      }
    }
    await report(testInfo, "F", { summary: summarizeOpens(measurements), measurements });
  });

  test("G multi-session tabs", async ({ browser, backend, apiClient, seedData }, testInfo) => {
    const seed = await seedMultiSessionTask(apiClient, seedData);
    const context = await newDesktopContext(browser, backend.frontendUrl);
    const page = await context.newPage();
    let open: OpenMeasurement | null = null;
    let steady: SteadyMeasurement | null = null;
    let panelsMounted = 0;
    try {
      open = await measureOpen(page, seed.taskId, seed.readyText, testInfo, "G-open");
      // Activate each session tab so every panel has been mounted at least once
      // and the steady trace captures any retained hidden-panel processing.
      for (const sessionId of seed.sessionIds) {
        const tab = page.getByTestId(`session-tab-${sessionId}`);
        if ((await tab.count()) === 0) continue;
        await tab.click();
        await page.locator(".chat-message-list:visible").first().waitFor({ timeout: OPEN_TASK_TIMEOUT });
      }
      panelsMounted = await page.locator("[data-testid='session-chat']").count();
      steady = await measureSteady(page, testInfo, "G-steady");
    } finally {
      await context.close();
    }
    await report(testInfo, "G", {
      openSummary: summarizeOpens([open]),
      open,
      steady,
      sessionsSeeded: seed.sessionIds.length,
      panelsMounted,
    });
  });

  test("H mobile (Pixel 5), text-heavy", async ({ browser, backend, apiClient, seedData }, testInfo) => {
    const seed = await seedTextHeavyTask(apiClient, seedData);
    const measurements: OpenMeasurement[] = [];
    for (let repeat = 1; repeat <= TRACE_REPEATS; repeat += 1) {
      const context = await browser.newContext({
        ...devices["Pixel 5"],
        baseURL: backend.frontendUrl,
      });
      const page = await context.newPage();
      try {
        measurements.push(
          await measureOpen(page, seed.taskId, seed.readyText, testInfo, `H-${repeat}`),
        );
      } finally {
        await context.close();
      }
    }
    await report(testInfo, "H", { summary: summarizeOpens(measurements), measurements });
  });
});
