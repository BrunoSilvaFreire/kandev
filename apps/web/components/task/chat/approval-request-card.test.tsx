import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, fireEvent } from "@testing-library/react";
import { useState } from "react";
import { ApprovalRequestCard } from "./approval-request-card";
import type { ApprovalDecision } from "@/lib/approval";

afterEach(() => {
  cleanup();
});

const approval = { subject: "task_plan" as const, title: "Plan revision 2" };

function renderCard(overrides: Partial<Parameters<typeof ApprovalRequestCard>[0]> = {}) {
  const onDecision = vi.fn<(decision: ApprovalDecision) => void>();
  function Wrapper() {
    const [feedback, setFeedback] = useState("");
    return (
      <ApprovalRequestCard
        approval={approval}
        summary="What changed since the last request."
        feedback={feedback}
        onFeedbackChange={setFeedback}
        commentCount={0}
        subjectEdited={false}
        isSubmitting={false}
        onOpenPlan={vi.fn()}
        onDecision={onDecision}
        {...overrides}
      />
    );
  }
  render(<Wrapper />);
  return { onDecision };
}

describe("ApprovalRequestCard", () => {
  it("renders the title, summary, badge and actions", () => {
    renderCard();
    expect(screen.getByTestId("approval-request-card")).toBeTruthy();
    expect(screen.getByText("Plan revision 2")).toBeTruthy();
    expect(screen.getByText("What changed since the last request.")).toBeTruthy();
    expect(screen.getByText("Approve")).toBeTruthy();
    expect(screen.getByText("Revise")).toBeTruthy();
    expect(screen.getByText("Reject")).toBeTruthy();
  });

  it("keeps Revise disabled until feedback is entered", () => {
    renderCard();
    const revise = screen.getByText("Revise").closest("button") as HTMLButtonElement;
    expect(revise.disabled).toBe(true);

    const textarea = screen.getByLabelText("Feedback (optional)");
    fireEvent.change(textarea, { target: { value: "please tighten the scope" } });

    expect(revise.disabled).toBe(false);
  });

  it("enables Revise when plan comments are pending", () => {
    renderCard({ commentCount: 2 });
    const revise = screen.getByText("Revise").closest("button") as HTMLButtonElement;
    expect(revise.disabled).toBe(false);
    expect(screen.getByTestId("approval-comment-count")).toBeTruthy();
  });

  it("shows the edited indicator and enables Revise on an edit", () => {
    renderCard({ subjectEdited: true });
    expect(screen.getByTestId("approval-edited")).toBeTruthy();
    const revise = screen.getByText("Revise").closest("button") as HTMLButtonElement;
    expect(revise.disabled).toBe(false);
  });

  it("reports the chosen decision", () => {
    const { onDecision } = renderCard({ subjectEdited: true });
    fireEvent.click(screen.getByText("Approve"));
    fireEvent.click(screen.getByText("Reject"));
    fireEvent.click(screen.getByText("Revise"));
    expect(onDecision).toHaveBeenNthCalledWith(1, "approve");
    expect(onDecision).toHaveBeenNthCalledWith(2, "reject");
    expect(onDecision).toHaveBeenNthCalledWith(3, "revise");
  });
});
