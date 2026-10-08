import { describe, expect, it } from "vitest";
import {
  STALE_REVIEW_MS,
  buildInboxTriage,
  selectUnacknowledgedTriageIds,
  type TriageBundleInput,
  type TriageTaskInput,
} from "./possible-question-triage";

const NOW = Date.parse("2026-10-06T12:00:00Z");

function task(overrides: Partial<TriageTaskInput>): TriageTaskInput {
  return { id: "t1", title: "Task", updated_at: "2026-10-06T11:00:00Z", ...overrides };
}

describe("buildInboxTriage", () => {
  it("ranks clarifications before stale review before possible questions", () => {
    const tasks: TriageTaskInput[] = [
      task({ id: "question", title: "Question", possible_question: true }),
      task({ id: "review", title: "Review", state: "REVIEW", interrupted: true }),
    ];
    const bundles: TriageBundleInput[] = [
      { task_id: "clar", session_id: "s", task_title: "Clarification" },
    ];
    const items = buildInboxTriage(tasks, bundles, NOW);
    expect(items.map((i) => i.kind)).toEqual([
      "clarification",
      "stale_review",
      "possible_question",
    ]);
  });

  it("deduplicates a task/session at its highest-priority kind", () => {
    const tasks: TriageTaskInput[] = [
      task({
        id: "t1",
        title: "Both",
        state: "REVIEW",
        interrupted: true,
        possible_question: true,
        primary_session_id: "s1",
      }),
    ];
    const bundles: TriageBundleInput[] = [{ task_id: "t1", session_id: "s1", task_title: "Both" }];
    const items = buildInboxTriage(tasks, bundles, NOW);
    expect(items).toHaveLength(1);
    expect(items[0].kind).toBe("clarification");
  });

  it("treats an old Review task as stale even without an interruption marker", () => {
    const old = new Date(NOW - STALE_REVIEW_MS - 1000).toISOString();
    const fresh = new Date(NOW - 1000).toISOString();
    const items = buildInboxTriage(
      [
        task({ id: "old", state: "REVIEW", updated_at: old }),
        task({ id: "fresh", state: "REVIEW", updated_at: fresh }),
      ],
      [],
      NOW,
    );
    expect(items.map((i) => i.taskId)).toEqual(["old"]);
  });

  it("ignores possible-question flags on non-waiting or already-moved tasks is caller-owned; flag drives the hint", () => {
    const items = buildInboxTriage(
      [task({ id: "t1", possible_question: true }), task({ id: "t2" })],
      [],
      NOW,
    );
    expect(items.map((i) => i.taskId)).toEqual(["t1"]);
  });

  it("keys a possible-question identity by turn so a new turn is a new identity", () => {
    const first = buildInboxTriage(
      [task({ id: "t1", possible_question: true, possible_question_turn_id: "turn-1" })],
      [],
      NOW,
    );
    const second = buildInboxTriage(
      [task({ id: "t1", possible_question: true, possible_question_turn_id: "turn-2" })],
      [],
      NOW,
    );
    expect(first[0].id).not.toBe(second[0].id);
    expect(first[0].kind).toBe("possible_question");
  });

  it("orders same-rank items newest first with a stable id tie-break", () => {
    const items = buildInboxTriage(
      [
        task({ id: "a", possible_question: true, updated_at: "2026-10-06T10:00:00Z" }),
        task({ id: "b", possible_question: true, updated_at: "2026-10-06T11:00:00Z" }),
      ],
      [],
      NOW,
    );
    expect(items.map((i) => i.taskId)).toEqual(["b", "a"]);
  });
});

describe("selectUnacknowledgedTriageIds", () => {
  it("returns only identities the operator has not acknowledged", () => {
    const items = buildInboxTriage(
      [task({ id: "a", possible_question: true }), task({ id: "b", possible_question: true })],
      [],
      NOW,
    );
    const acknowledged = new Set([items[0].id]);
    expect(selectUnacknowledgedTriageIds(items, acknowledged)).toEqual([items[1].id]);
  });
});
