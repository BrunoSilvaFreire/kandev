import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const state = vi.hoisted(() => ({
  tasks: [] as unknown[],
  snapshots: {} as Record<string, { tasks?: unknown[] }>,
  bundles: [] as unknown[],
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (value: unknown) => unknown) =>
    selector({
      kanban: { tasks: state.tasks },
      kanbanMulti: { snapshots: state.snapshots },
    }),
}));

vi.mock("@/lib/state/slices/needs-you-inbox/selectors", () => ({
  selectNeedsYouInboxBundles: () => state.bundles,
}));

import { resetAcknowledgedTriageForTests } from "@/lib/inbox-triage/acknowledged-store";
import { TRIAGE_CLOCK_MS, useInboxTriageNudge } from "./use-inbox-triage";

function questionTask(turnId: string) {
  return {
    id: "t1",
    title: "Question task",
    state: "WAITING_FOR_INPUT",
    primarySessionId: "s1",
    updatedAt: "2026-10-06T11:00:00Z",
    statusSummary: { possible_question: true, possible_question_turn_id: turnId },
  };
}

describe("useInboxTriageNudge", () => {
  beforeEach(() => {
    window.localStorage.clear();
    resetAcknowledgedTriageForTests();
    state.tasks = [];
    state.snapshots = {};
    state.bundles = [];
  });

  afterEach(() => {
    resetAcknowledgedTriageForTests();
  });

  it("counts an unacknowledged possible question, then clears it on acknowledgement", () => {
    state.tasks = [questionTask("turn-1")];
    const { result } = renderHook(() => useInboxTriageNudge());
    expect(result.current.additionalCount).toBe(1);
    expect(result.current.newCount).toBe(1);

    act(() => result.current.acknowledgeAll());
    expect(result.current.additionalCount).toBe(0);
    expect(result.current.newCount).toBe(0);
  });

  it("re-surfaces the nudge when the same session produces a new turn", () => {
    state.tasks = [questionTask("turn-1")];
    const { result, rerender } = renderHook(() => useInboxTriageNudge());
    act(() => result.current.acknowledgeAll());
    expect(result.current.additionalCount).toBe(0);

    state.tasks = [questionTask("turn-2")];
    rerender();
    expect(result.current.additionalCount).toBe(1);
  });

  it("does not add bundle-backed clarifications to the additional count", () => {
    state.bundles = [{ task_id: "t1", session_id: "s1", task_title: "Clarification" }];
    const { result } = renderHook(() => useInboxTriageNudge());
    expect(result.current.items.map((item) => item.kind)).toEqual(["clarification"]);
    expect(result.current.newCount).toBe(1);
    expect(result.current.additionalCount).toBe(0);
  });

  it("ages a review into stale from the clock alone, without a store event", () => {
    vi.useFakeTimers();
    try {
      const enteredAt = Date.now() - 24 * 60 * 60 * 1000 + TRIAGE_CLOCK_MS / 2;
      state.tasks = [
        {
          id: "t1",
          title: "Review task",
          state: "REVIEW",
          reviewEpisodeId: "r1",
          reviewEnteredAt: new Date(enteredAt).toISOString(),
          // A recent edit must not keep the review fresh.
          updatedAt: new Date().toISOString(),
        },
      ];
      const { result } = renderHook(() => useInboxTriageNudge());
      expect(result.current.items.map((item) => item.kind)).toEqual(["review"]);

      act(() => {
        vi.advanceTimersByTime(TRIAGE_CLOCK_MS);
      });
      expect(result.current.items.map((item) => item.kind)).toEqual(["stale_review"]);
    } finally {
      vi.useRealTimers();
    }
  });

  it("keeps one acknowledged interruption quiet across repeated reconciliation", () => {
    const interrupted = (episodeId: string) => ({
      id: "t1",
      title: "Interrupted task",
      state: "IN_PROGRESS",
      interrupted: true,
      interruption: { episode_id: episodeId, reason: "backend_restart", session_id: "s1" },
    });
    state.tasks = [interrupted("e1")];
    const { result, rerender } = renderHook(() => useInboxTriageNudge());
    expect(result.current.items.map((item) => item.kind)).toEqual(["interrupted"]);
    act(() => result.current.acknowledgeAll());

    // A second restart re-publishes the same episode: no new nudge.
    state.tasks = [interrupted("e1")];
    rerender();
    expect(result.current.additionalCount).toBe(0);

    // A later, distinct episode nudges again.
    state.tasks = [interrupted("e2")];
    rerender();
    expect(result.current.additionalCount).toBe(1);
  });
});
