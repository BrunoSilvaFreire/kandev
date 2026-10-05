import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { WorkflowTransitionSummary } from "./workflow-transition-summary";
import type { WorkflowStepperStep } from "./workflow-step-disclosure";

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      taskSessions: { items: { "sess-9": { id: "sess-9", name: "Plan session" } } },
      agentProfiles: { items: [{ id: "profile-1", label: "Claude" }] },
    }),
}));

afterEach(() => cleanup());

function step(
  id: string,
  position: number,
  events?: WorkflowStepperStep["events"],
): WorkflowStepperStep {
  return { id, name: `Step ${id}`, color: "", position, events };
}

const steps = [
  step("a", 0, { on_turn_complete: [{ type: "move_to_next" }] }),
  step("b", 1, { on_turn_complete: [{ type: "move_to_previous" }] }),
];

describe("WorkflowTransitionSummary", () => {
  it("renders each possible edge once with its committed count, including zero", () => {
    render(
      <WorkflowTransitionSummary
        steps={steps}
        currentStepId="a"
        summary={{ counts: { "a->b": 4 }, visits: {} }}
      />,
    );
    const edges = screen.getAllByTestId("workflow-transition-edge");
    expect(edges).toHaveLength(2);
    const aToB = edges.find((edge) => edge.querySelector("span")?.textContent === "Step a");
    expect(aToB?.textContent).toContain("4");
    const bToA = edges.find((edge) => edge.querySelector("span")?.textContent === "Step b");
    // Allowed-but-never-taken edges still render a zero count.
    expect(bToA?.textContent).toContain("0");
  });

  it("marks current-step manual destinations as dashed", () => {
    render(
      <WorkflowTransitionSummary
        steps={[step("a", 0, undefined), step("b", 1, undefined)]}
        currentStepId="a"
        summary={{ counts: {}, visits: {} }}
      />,
    );
    // a is current but not allow_manual_move, so no manual edges here.
    expect(screen.queryAllByTestId("workflow-transition-edge")).toHaveLength(0);
  });

  it("lists each visit's destination sessions and activates one on click", () => {
    const onOpenSession = vi.fn();
    render(
      <WorkflowTransitionSummary
        steps={steps}
        currentStepId="a"
        summary={{
          counts: {},
          visits: {
            b: {
              count: 3,
              sessions: [
                {
                  session_id: "sess-9",
                  agent_profile_id: "profile-1",
                  occurred_at: "2026-09-25T10:00:00Z",
                },
              ],
            },
          },
        }}
        onOpenSession={onOpenSession}
      />,
    );
    expect(screen.getByTestId("workflow-step-visit-b").textContent).toContain("3");
    expect(screen.getByTestId("workflow-step-visit-session-b-list").textContent).toContain(
      "Plan session",
    );
    fireEvent.click(screen.getByTestId("workflow-step-visit-session-b-sess-9"));
    expect(onOpenSession).toHaveBeenCalledWith("sess-9");
  });

  it("opens the step-filtered Task History from a visited step", () => {
    const onOpenHistory = vi.fn();
    render(
      <WorkflowTransitionSummary
        steps={steps}
        currentStepId="a"
        summary={{
          counts: {},
          visits: {
            b: {
              count: 1,
              sessions: [{ session_id: "sess-9", occurred_at: "2026-09-25T10:00:00Z" }],
            },
          },
        }}
        onOpenHistory={onOpenHistory}
      />,
    );
    fireEvent.click(screen.getByTestId("workflow-step-visit-history-b"));
    expect(onOpenHistory).toHaveBeenCalledWith("b");
  });
});
