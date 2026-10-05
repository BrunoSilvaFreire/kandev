import { describe, expect, it } from "vitest";
import { deriveWorkflowTransitionEdges } from "./workflow-transition-edges";
import type { WorkflowStepperStep } from "@/components/task/workflow-step-disclosure";

function step(
  id: string,
  position: number,
  events?: WorkflowStepperStep["events"],
  allowManual = false,
): WorkflowStepperStep {
  return { id, name: id, color: "", position, events, allow_manual_move: allowManual };
}

describe("deriveWorkflowTransitionEdges", () => {
  it("resolves relative next/previous and explicit move_to_step", () => {
    const steps = [
      step("a", 0, { on_turn_complete: [{ type: "move_to_next" }] }),
      step("b", 1, { on_turn_complete: [{ type: "move_to_previous" }] }),
      step("c", 2, { on_enter: [{ type: "move_to_step", config: { step_id: "a" } }] }),
    ];
    const edges = deriveWorkflowTransitionEdges(steps, null);
    const keys = edges.map((edge) => `${edge.fromStepId}->${edge.toStepId}`).sort();
    expect(keys).toEqual(["a->b", "b->a", "c->a"]);
    expect(edges.every((edge) => !edge.manual)).toBe(true);
  });

  it("collapses duplicate configured actions onto one pair and keeps reciprocal edges", () => {
    const steps = [
      step("a", 0, {
        on_turn_complete: [{ type: "move_to_next" }, { type: "move_to_next" }],
      }),
      step("b", 1, { on_turn_complete: [{ type: "move_to_previous" }] }),
    ];
    const edges = deriveWorkflowTransitionEdges(steps, null);
    const pairs = edges.map((edge) => `${edge.fromStepId}->${edge.toStepId}`).sort();
    expect(pairs).toEqual(["a->b", "b->a"]);
  });

  it("adds manual destinations only from the current step", () => {
    const steps = [step("a", 0), step("b", 1, undefined, true), step("c", 2)];
    const edges = deriveWorkflowTransitionEdges(steps, "b");
    const manual = edges
      .filter((edge) => edge.manual)
      .map((edge) => `${edge.fromStepId}->${edge.toStepId}`)
      .sort();
    expect(manual).toEqual(["b->a", "b->c"]);
    expect(edges.some((edge) => edge.fromStepId === "a")).toBe(false);
  });

  it("prefers an automatic edge over a manual duplicate", () => {
    const steps = [
      step("a", 0),
      step("b", 1, { on_turn_complete: [{ type: "move_to_next" }] }, true),
      step("c", 2),
    ];
    const edges = deriveWorkflowTransitionEdges(steps, "b");
    const bToC = edges.find((edge) => edge.fromStepId === "b" && edge.toStepId === "c");
    expect(bToC?.manual).toBe(false);
    expect(edges.find((edge) => edge.fromStepId === "b" && edge.toStepId === "a")?.manual).toBe(
      true,
    );
  });

  it("resolves a legacy move_to_step step_position against position", () => {
    const steps = [
      step("a", 0),
      step("b", 1, { on_turn_start: [{ type: "move_to_step", config: { step_position: 0 } }] }),
    ];
    const edges = deriveWorkflowTransitionEdges(steps, null);
    expect(edges.map((edge) => `${edge.fromStepId}->${edge.toStepId}`)).toEqual(["b->a"]);
  });

  it("ignores self-targets and unknown positions", () => {
    const steps = [
      step("a", 0, { on_turn_complete: [{ type: "move_to_step", config: { step_id: "a" } }] }),
      step("b", 1, { on_turn_complete: [{ type: "move_to_step", config: { step_position: 9 } }] }),
    ];
    expect(deriveWorkflowTransitionEdges(steps, null)).toEqual([]);
  });
});
