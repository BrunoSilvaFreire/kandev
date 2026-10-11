import { describe, expect, it } from "vitest";
import {
  STALE_REVIEW_MS,
  buildInboxTriage,
  selectUnacknowledgedTriageIds,
  type TriageBundleInput,
  type TriageTaskInput,
} from "./possible-question-triage";

const NOW = Date.parse("2026-10-06T12:00:00Z");
const OLD = new Date(NOW - STALE_REVIEW_MS - 1000).toISOString();
const FRESH = new Date(NOW - 1000).toISOString();

function task(overrides: Partial<TriageTaskInput>): TriageTaskInput {
  return { id: "t1", title: "Task", updated_at: "2026-10-06T11:00:00Z", ...overrides };
}

function episode(id: string, reason = "backend_restart", session = "s1") {
  return { episode_id: id, reason, session_id: session };
}

describe("buildInboxTriage", () => {
  it("ranks every kind in a deterministic order", () => {
    const tasks: TriageTaskInput[] = [
      task({ id: "question", possible_question: true }),
      task({ id: "blocked", blocked: true }),
      task({ id: "review", state: "REVIEW", review_entered_at: FRESH }),
      task({ id: "stale", state: "REVIEW", review_entered_at: OLD }),
      task({ id: "lost", interrupted: true, interruption: episode("e1") }),
    ];
    const bundles: TriageBundleInput[] = [
      { task_id: "clar", session_id: "s", task_title: "Clarification" },
    ];
    expect(buildInboxTriage(tasks, bundles, NOW).map((i) => i.kind)).toEqual([
      "clarification",
      "interrupted",
      "stale_review",
      "possible_question",
      "review",
      "dependency",
    ]);
  });

  it("shows one row per task and lets a clarification win", () => {
    const tasks = [
      task({
        state: "REVIEW",
        interrupted: true,
        interruption: episode("e1"),
        possible_question: true,
        blocked: true,
        primary_session_id: "s1",
      }),
    ];
    const bundles: TriageBundleInput[] = [{ task_id: "t1", session_id: "s2", task_title: "Both" }];
    const items = buildInboxTriage(tasks, bundles, NOW);
    expect(items).toHaveLength(1);
    expect(items[0].kind).toBe("clarification");
  });

  it("presents an interrupted Review task as resumable work, never a stale review", () => {
    const items = buildInboxTriage(
      [
        task({
          state: "REVIEW",
          review_entered_at: OLD,
          interrupted: true,
          interruption: episode("e1", "execution_lost", "s9"),
        }),
      ],
      [],
      NOW,
    );
    expect(items).toHaveLength(1);
    expect(items[0]).toMatchObject({
      kind: "interrupted",
      reason: "execution_lost",
      sessionId: "s9",
    });
  });

  it("keys an interruption by episode so a later episode is a new identity", () => {
    const build = (id: string) =>
      buildInboxTriage([task({ interrupted: true, interruption: episode(id) })], [], NOW)[0].id;
    expect(build("e1")).toBe(build("e1"));
    expect(build("e1")).not.toBe(build("e2"));
  });

  it("labels an unattributed legacy marker as unknown without inventing a session", () => {
    const [item] = buildInboxTriage(
      [task({ interrupted: true, interrupted_at: "2026-10-01T00:00:00Z" })],
      [],
      NOW,
    );
    expect(item).toMatchObject({ kind: "interrupted", reason: "interrupted_unknown" });
    expect(item.sessionId).toBeNull();
    expect(item.id).toContain("legacy:2026-10-01T00:00:00Z");
  });

  it("ignores a stale episode once the task reports it is no longer interrupted", () => {
    expect(
      buildInboxTriage([task({ interrupted: false, interruption: episode("e1") })], [], NOW),
    ).toEqual([]);
  });
});

describe("buildInboxTriage review age and identity", () => {
  it("measures Review age from the entry time, never the last edit", () => {
    const items = buildInboxTriage(
      [
        // Entered long ago but edited a second ago: still stale.
        task({ id: "edited", state: "REVIEW", review_entered_at: OLD, updated_at: FRESH }),
        // Entered a second ago but last edited long ago: still fresh.
        task({ id: "entered", state: "REVIEW", review_entered_at: FRESH, updated_at: OLD }),
        // No entry time (older server): a review, never stale by updated_at.
        task({ id: "unknown", state: "REVIEW", updated_at: OLD }),
      ],
      [],
      NOW,
    );
    expect(Object.fromEntries(items.map((i) => [i.taskId, i.kind]))).toEqual({
      edited: "stale_review",
      entered: "review",
      unknown: "review",
    });
  });

  it("ages a review into stale from time alone and gives it a new identity", () => {
    const tasks = [
      task({ state: "REVIEW", review_episode_id: "r1", review_entered_at: "2026-10-06T00:00:00Z" }),
    ];
    const [before] = buildInboxTriage(tasks, [], NOW);
    const [after] = buildInboxTriage(tasks, [], NOW + STALE_REVIEW_MS);
    expect(before.kind).toBe("review");
    expect(after.kind).toBe("stale_review");
    expect(after.id).not.toBe(before.id);
  });

  it("keys a review by its episode so re-entry is a new identity", () => {
    const build = (id: string) =>
      buildInboxTriage(
        [task({ state: "REVIEW", review_episode_id: id, review_entered_at: FRESH })],
        [],
        NOW,
      )[0].id;
    expect(build("r1")).not.toBe(build("r2"));
  });

  it("keys a possible-question identity by turn so a new turn is a new identity", () => {
    const build = (turn: string) =>
      buildInboxTriage(
        [task({ possible_question: true, possible_question_turn_id: turn })],
        [],
        NOW,
      )[0];
    expect(build("turn-1").id).not.toBe(build("turn-2").id);
    expect(build("turn-1").kind).toBe("possible_question");
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

  it("never nudges for a fresh review or a dependency wait", () => {
    const items = buildInboxTriage(
      [
        task({ id: "review", state: "REVIEW", review_entered_at: FRESH }),
        task({ id: "blocked", blocked: true }),
        task({ id: "stale", state: "REVIEW", review_entered_at: OLD }),
      ],
      [],
      NOW,
    );
    expect(items).toHaveLength(3);
    expect(selectUnacknowledgedTriageIds(items, new Set())).toEqual([
      items.find((i) => i.taskId === "stale")?.id,
    ]);
  });
});
