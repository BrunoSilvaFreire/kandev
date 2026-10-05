import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

import { ContinuationRecoveryCard } from "./continuation-recovery-card";

afterEach(cleanup);

describe("ContinuationRecoveryCard", () => {
  it("renders translated labels for the kind instead of the server's English copy", () => {
    render(
      <ContinuationRecoveryCard
        recovery={{ stamp: "s", case: "cold", reason: "reset_failed" }}
        questionId="continuation_recovery"
        isSubmitting={false}
        onSubmitAnswer={() => {}}
      />,
    );

    expect(screen.getByTestId("continuation-recovery-card")).toBeDefined();
    expect(screen.getByText("task:continuationRecoveryTitle")).toBeDefined();
    expect(screen.getByText("task:continuationRecoveryReasonResetFailed")).toBeDefined();
    expect(screen.getByText("task:continuationRecoveryRetry")).toBeDefined();
    expect(screen.getByText("task:continuationRecoveryContinue")).toBeDefined();
  });

  it("submits the selected decision for the fixed question", () => {
    const onSubmitAnswer = vi.fn();
    render(
      <ContinuationRecoveryCard
        recovery={{ stamp: "s", case: "cold", reason: "extraction_failed" }}
        questionId="continuation_recovery"
        isSubmitting={false}
        onSubmitAnswer={onSubmitAnswer}
      />,
    );

    fireEvent.click(screen.getByText("task:continuationRecoveryRetry"));
    expect(onSubmitAnswer).toHaveBeenCalledWith({
      question_id: "continuation_recovery",
      selected_options: ["retry"],
    });
  });
});
