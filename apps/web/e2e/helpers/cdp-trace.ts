import type { CDPSession, Page, TestInfo } from "@playwright/test";

import { dwell } from "./causal-waits";

/**
 * Reusable Chrome DevTools Protocol (CDP) trace plumbing shared by the opt-in
 * performance specs. The tracing categories and the `ReturnAsStream` readback
 * are copied from the animation-performance control so both specs attribute
 * cost the same way; only the metric reduction differs per spec.
 *
 * These helpers are deliberately dependency-free: the trace is captured by a
 * Playwright CDP session, attached to the report, and reduced in-process. No
 * profiler package is required.
 */

export const TRACE_CATEGORIES = [
  "devtools.timeline",
  "disabled-by-default-devtools.timeline",
  "disabled-by-default-devtools.timeline.invalidationTracking",
  "blink.user_timing",
].join(",");

export type TraceEvent = {
  name?: string;
  args?: unknown;
  dur?: number;
  ts?: number;
};

export type TraceMetric = {
  count: number;
  durationMs: number;
};

export type NumericSummary = {
  min: number;
  median: number;
  max: number;
};

export type TraceCaptureOptions = {
  /** Disable script execution for the trace window (control arms only). */
  disableScriptExecution?: boolean;
  /** Trace categories; defaults to {@link TRACE_CATEGORIES}. */
  categories?: string;
};

/**
 * Capture one fixed-length trace window and return its raw events.
 *
 * The window length is a wall-clock wait because the browser publishes no event
 * when the window ends — it is the sampling clock, not a synchronization wait.
 * The raw trace is attached to the Playwright report as `<label>.json`.
 */
export async function captureTraceEvents(
  page: Page,
  testInfo: TestInfo,
  label: string,
  windowMs: number,
  options: TraceCaptureOptions = {},
): Promise<TraceEvent[]> {
  const client = await page.context().newCDPSession(page);
  try {
    await startTrace(client, options);
    await dwell(
      windowMs,
      "clock-separation",
      "the fixed acceptance trace window has no completion event",
    );
    return await stopTrace(client, testInfo, label);
  } catch (error) {
    await client
      .send("Emulation.setScriptExecutionDisabled", { value: false })
      .catch(() => undefined);
    await client.detach().catch(() => undefined);
    throw error;
  }
}

/**
 * Begin a CDP trace. Pair with {@link stopTrace}; the caller controls how long
 * the trace runs (open-latency measurements stop when the transcript is ready,
 * not after a fixed window).
 */
export async function startTrace(
  client: CDPSession,
  options: TraceCaptureOptions = {},
): Promise<void> {
  await client.send("Emulation.setScriptExecutionDisabled", {
    value: options.disableScriptExecution ?? true,
  });
  await dwell(
    1_000,
    "clock-separation",
    "the browser does not publish an event when pending application tasks are drained",
  );
  await client.send("Tracing.start", {
    categories: options.categories ?? TRACE_CATEGORIES,
    transferMode: "ReturnAsStream",
  });
}

/**
 * End a trace started with {@link startTrace}, attach the raw trace to the
 * Playwright report, and return its events. Re-enables script execution and
 * detaches the CDP session.
 */
export async function stopTrace(
  client: CDPSession,
  testInfo: TestInfo,
  label: string,
): Promise<TraceEvent[]> {
  const stream = traceStream(client);
  await client.send("Tracing.end");
  let trace: string;
  try {
    trace = await readTraceStream(client, await stream);
  } finally {
    await client
      .send("Emulation.setScriptExecutionDisabled", { value: false })
      .catch(() => undefined);
    await client.detach().catch(() => undefined);
  }
  await testInfo.attach(`${label}.json`, {
    body: Buffer.from(trace),
    contentType: "application/json",
  });
  return (JSON.parse(trace) as { traceEvents: TraceEvent[] }).traceEvents;
}

function traceStream(client: CDPSession): Promise<string> {
  return new Promise((resolve) => {
    client.once("Tracing.tracingComplete", (event) => resolve(event.stream ?? ""));
  });
}

async function readTraceStream(client: CDPSession, stream: string): Promise<string> {
  let trace = "";
  let eof = false;
  while (!eof) {
    const chunk = await client.send("IO.read", { handle: stream });
    trace += chunk.base64Encoded ? Buffer.from(chunk.data, "base64").toString("utf8") : chunk.data;
    eof = chunk.eof;
  }
  await client.send("IO.close", { handle: stream });
  return trace;
}

/** Count and total (ms) of the trace events with this exact name. */
export function traceMetricNamed(events: TraceEvent[], name: string): TraceMetric {
  const matchingEvents = events.filter((event) => event.name === name);
  return {
    count: matchingEvents.length,
    durationMs: matchingEvents.reduce((total, event) => total + (event.dur ?? 0), 0) / 1_000,
  };
}

/** min/median/max over a numeric series. Throws on an empty series. */
export function summarizeNumbers(values: number[]): NumericSummary {
  if (values.length === 0) throw new Error("Cannot summarize an empty value set");
  const sorted = [...values].sort((left, right) => left - right);
  const middle = Math.floor(sorted.length / 2);
  const median =
    sorted.length % 2 === 0 ? (sorted[middle - 1] + sorted[middle]) / 2 : sorted[middle];
  return { min: sorted[0], median, max: sorted[sorted.length - 1] };
}

/** Reduce a per-run trace metric series into count/duration summaries. */
export function summarizeTraceMetric(metrics: TraceMetric[]): {
  count: NumericSummary;
  durationMs: NumericSummary;
} {
  return {
    count: summarizeNumbers(metrics.map((metric) => metric.count)),
    durationMs: summarizeNumbers(metrics.map((metric) => metric.durationMs)),
  };
}

export function parsePositiveInteger(value: string): number {
  const parsed = Number.parseInt(value, 10);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : 1;
}

// ---------------------------------------------------------------------------
// Performance counters (heap, nodes, layout, recalc)
// ---------------------------------------------------------------------------

export type PerformanceMetricSnapshot = Record<string, number>;

/**
 * Read `Performance.getMetrics` as a name→value map. When `collectGarbage` is
 * set, force a GC first so `JSHeapUsedSize` reflects live objects rather than
 * uncollected garbage.
 */
export async function readPerformanceMetrics(
  client: CDPSession,
  options: { collectGarbage?: boolean } = {},
): Promise<PerformanceMetricSnapshot> {
  if (options.collectGarbage) {
    await client.send("HeapProfiler.collectGarbage");
  }
  await client.send("Performance.enable");
  const { metrics } = await client.send("Performance.getMetrics");
  const snapshot: PerformanceMetricSnapshot = {};
  for (const metric of metrics) snapshot[metric.name] = metric.value;
  return snapshot;
}

// ---------------------------------------------------------------------------
// Network accounting
// ---------------------------------------------------------------------------

export type NetworkEntry = {
  url: string;
  count: number;
  encodedDataLength: number;
};

export type NetworkSummary = {
  requestCount: number;
  finishedCount: number;
  encodedDataLength: number;
  /** Per-URL request count and transferred bytes, largest transfer first. */
  byUrl: NetworkEntry[];
};

/**
 * Start counting requests and transferred bytes over a CDP session. Returns a
 * stop function that detaches the listeners and resolves the totals plus a
 * per-URL breakdown for attribution.
 */
export async function startNetworkCapture(client: CDPSession): Promise<() => NetworkSummary> {
  const summary: NetworkSummary = {
    requestCount: 0,
    finishedCount: 0,
    encodedDataLength: 0,
    byUrl: [],
  };
  const urlByRequestId = new Map<string, string>();
  const entryByUrl = new Map<string, NetworkEntry>();
  const onRequest = (event: { requestId: string; request?: { url?: string } }) => {
    summary.requestCount += 1;
    const url = event.request?.url;
    if (!url) return;
    urlByRequestId.set(event.requestId, url);
    const entry = entryByUrl.get(url) ?? { url, count: 0, encodedDataLength: 0 };
    entry.count += 1;
    entryByUrl.set(url, entry);
  };
  const onFinished = (event: { requestId: string; encodedDataLength?: number }) => {
    summary.finishedCount += 1;
    const bytes = event.encodedDataLength ?? 0;
    summary.encodedDataLength += bytes;
    const url = urlByRequestId.get(event.requestId);
    if (!url) return;
    const entry = entryByUrl.get(url);
    if (entry) entry.encodedDataLength += bytes;
  };
  client.on("Network.requestWillBeSent", onRequest);
  client.on("Network.loadingFinished", onFinished);
  await client.send("Network.enable");
  return () => {
    client.off("Network.requestWillBeSent", onRequest);
    client.off("Network.loadingFinished", onFinished);
    summary.byUrl = [...entryByUrl.values()].sort(
      (left, right) => right.encodedDataLength - left.encodedDataLength,
    );
    return { ...summary, byUrl: [...summary.byUrl] };
  };
}

// ---------------------------------------------------------------------------
// In-page PerformanceObserver: long tasks and largest contentful paint
// ---------------------------------------------------------------------------

export type LongTaskSummary = {
  count: number;
  maxMs: number;
  totalMs: number;
  p95Ms: number;
};

export type PerformanceObserverSummary = {
  longTasks: LongTaskSummary;
  largestContentfulPaintMs: number | null;
};

type ObserverWindow = Window & {
  __kandevLongTasks?: number[];
  __kandevLcp?: number[];
};

/**
 * Register long-task and largest-contentful-paint observers before navigation.
 * Call this **before** `page.goto()` so buffered entries from the initial
 * render are captured.
 */
export async function installPerformanceObservers(page: Page): Promise<void> {
  await page.addInitScript(() => {
    const observerWindow = window as ObserverWindow;
    observerWindow.__kandevLongTasks = [];
    observerWindow.__kandevLcp = [];
    try {
      new PerformanceObserver((list) => {
        for (const entry of list.getEntries()) observerWindow.__kandevLongTasks?.push(entry.duration);
      }).observe({ type: "longtask", buffered: true });
    } catch {
      // The browser may not support the longtask entry type; the read side
      // then reports zero long tasks rather than failing the run.
    }
    try {
      new PerformanceObserver((list) => {
        for (const entry of list.getEntries()) observerWindow.__kandevLcp?.push(entry.startTime);
      }).observe({ type: "largest-contentful-paint", buffered: true });
    } catch {
      // See above.
    }
  });
}

export async function readPerformanceObservers(page: Page): Promise<PerformanceObserverSummary> {
  const raw = await page.evaluate(() => {
    const observerWindow = window as ObserverWindow;
    return {
      longTasks: observerWindow.__kandevLongTasks ?? [],
      lcp: observerWindow.__kandevLcp ?? [],
    };
  });
  return {
    longTasks: summarizeLongTasks(raw.longTasks),
    largestContentfulPaintMs: raw.lcp.length > 0 ? Math.max(...raw.lcp) : null,
  };
}

function summarizeLongTasks(durations: number[]): LongTaskSummary {
  if (durations.length === 0) return { count: 0, maxMs: 0, totalMs: 0, p95Ms: 0 };
  const sorted = [...durations].sort((left, right) => left - right);
  const p95Index = Math.min(sorted.length - 1, Math.ceil(sorted.length * 0.95) - 1);
  return {
    count: sorted.length,
    maxMs: sorted[sorted.length - 1],
    totalMs: sorted.reduce((total, duration) => total + duration, 0),
    p95Ms: sorted[Math.max(0, p95Index)],
  };
}
