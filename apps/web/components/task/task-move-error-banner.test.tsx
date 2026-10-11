import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { TransitionFailureRecord } from "@/lib/tasks/transition-failure";

const openPlan = vi.fn();
vi.mock("@/hooks/domains/task/use-task-transition-failure", () => ({
  useOpenTaskPlan: () => openPlan,
}));
vi.mock("@/hooks/use-compact-task-chrome", () => ({ useTouchDrawer: () => false }));
vi.mock("@/components/state-provider", () => ({ useAppStore: () => null }));

import { TaskTransitionFailureBanner } from "./task-move-error-banner";

function record(overrides: Partial<TransitionFailureRecord> = {}): TransitionFailureRecord {
  return {
    task_id: "t1",
    attempt_id: "signal-1",
    source: "auto_advance",
    created_at: "2026-10-11T00:00:00Z",
    failure: {
      code: "task_completion_gate_blocked",
      message: "Task completion requirements are not satisfied.",
      actionability: "user",
      reasons: [
        { rule: "completion_gate.plan_stale", action: "x", cta: { kind: "open_plan" } },
        {
          rule: "completion_gate.criterion_unverified",
          criterion_id: "I3",
          description: "Inbox is truthful",
          action: "x",
        },
      ],
    },
    ...overrides,
  };
}

describe("TaskTransitionFailureBanner", () => {
  afterEach(cleanup);

  it("says the accepted signal did not advance the task and why", () => {
    render(<TaskTransitionFailureBanner record={record()} />);
    const banner = screen.getByTestId("task-transition-failure-banner");
    expect(banner.textContent).toContain("did not advance");
    expect(banner.textContent).toContain("Completion requirements are not met.");

    fireEvent.click(screen.getByTestId("transition-failure-details-toggle"));
    const reasons = screen.getAllByTestId("transition-failure-reason");
    expect(reasons.map((reason) => reason.getAttribute("data-rule"))).toEqual([
      "completion_gate.plan_stale",
      "completion_gate.criterion_unverified",
    ]);
    expect(reasons[1].textContent).toContain("Criterion I3: Inbox is truthful");

    fireEvent.click(screen.getByTestId("transition-failure-open-plan"));
    expect(openPlan).toHaveBeenCalledTimes(1);
  });

  it("labels a deferred move and a system fault distinctly", () => {
    render(
      <TaskTransitionFailureBanner
        record={record({
          source: "deferred_move",
          failure: {
            code: "task_transition_internal_failure",
            message: "",
            actionability: "system",
            incident_id: "inc-42",
            reasons: [{ rule: "system.internal", action: "x" }],
          },
        })}
      />,
    );
    expect(screen.getByTestId("task-transition-failure-banner").textContent).toContain(
      "queued move did not apply",
    );
    fireEvent.click(screen.getByTestId("transition-failure-details-toggle"));
    expect(screen.getByTestId("transition-failure-actionability").textContent).toBe(
      "System problem",
    );
    expect(screen.getByTestId("transition-failure-incident").textContent).toContain("inc-42");
    expect(screen.queryByTestId("transition-failure-open-plan")).toBeNull();
  });
});
