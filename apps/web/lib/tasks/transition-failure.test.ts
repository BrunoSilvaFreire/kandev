import { describe, expect, it } from "vitest";
import { ApiError } from "@/lib/api/client";
import {
  getTransitionFailure,
  parseTransitionFailure,
  parseTransitionFailureRecord,
  transitionFailureHeadlineKey,
  transitionReasonKey,
} from "./transition-failure";

const gateFailure = {
  code: "task_completion_gate_blocked",
  message: "Task completion requirements are not satisfied.",
  actionability: "user",
  remediation: "Verify these increments with evidence.",
  reasons: [
    {
      rule: "completion_gate.plan_stale",
      action: "Request approval",
      cta: { kind: "open_plan" },
    },
    {
      rule: "completion_gate.criterion_unverified",
      criterion_id: "I1",
      description: "Ships the diff",
      action: "Verify",
    },
    { bogus: true },
  ],
};

describe("transition failure envelope", () => {
  it("parses reasons in order and drops malformed entries", () => {
    const failure = parseTransitionFailure(gateFailure);
    expect(failure?.reasons.map((reason) => reason.rule)).toEqual([
      "completion_gate.plan_stale",
      "completion_gate.criterion_unverified",
    ]);
    expect(failure?.reasons[0].cta).toEqual({ kind: "open_plan", target: undefined });
    expect(failure?.reasons[1].criterion_id).toBe("I1");
  });

  it("reads the envelope from a rejected move and ignores older servers", () => {
    const withEnvelope = new ApiError("conflict", 409, {
      code: gateFailure.code,
      failure: gateFailure,
    });
    expect(getTransitionFailure(withEnvelope)?.code).toBe("task_completion_gate_blocked");
    const olderServer = new ApiError("conflict", 409, { code: "task_move_wip_limit" });
    expect(getTransitionFailure(olderServer)).toBeNull();
    expect(getTransitionFailure(new Error("network"))).toBeNull();
  });

  it("treats anything but an explicit system fault as user-repairable", () => {
    expect(parseTransitionFailure({ code: "x", reasons: [] })?.actionability).toBe("user");
    expect(
      parseTransitionFailure({
        code: "x",
        reasons: [],
        actionability: "system",
        incident_id: "i-1",
      }),
    ).toMatchObject({ actionability: "system", incident_id: "i-1" });
  });

  it("parses an asynchronous failure record", () => {
    const record = parseTransitionFailureRecord({
      task_id: "t1",
      attempt_id: "move-1",
      source: "deferred_move",
      created_at: "2026-10-11T00:00:00Z",
      failure: gateFailure,
    });
    expect(record).toMatchObject({ task_id: "t1", source: "deferred_move" });
    expect(parseTransitionFailureRecord({ task_id: "t1" })).toBeNull();
    expect(parseTransitionFailureRecord(null)).toBeNull();
  });

  it("maps known codes and rules to copy and falls back for unknown ones", () => {
    expect(transitionFailureHeadlineKey("task_move_wip_limit")).toBe("task:taskMoveErrorWipLimit");
    expect(transitionFailureHeadlineKey("from_a_newer_server")).toBeNull();
    expect(transitionReasonKey("move.wip_limit")).toBe("task:transitionReasonWipLimit");
    expect(transitionReasonKey("from_a_newer_server")).toBe("task:transitionReasonGeneric");
  });
});
