// A refused move explains itself: headline, expandable reasons naming the
// unmet criterion, and no fabricated success. The mobile-chrome sibling lives
// in mobile-transition-failure-diagnostics.spec.ts.
import { test, expect } from "../../fixtures/test-base";
import {
  CRITERION_DESCRIPTION,
  attemptGateBlockedMove,
} from "./transition-failure-diagnostics-helpers";

test.describe("Transition failure diagnostics", () => {
  test("explains a completion-gate refusal with expandable reasons", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { taskId } = await attemptGateBlockedMove(testPage, apiClient, seedData);

    const banner = testPage.getByTestId("task-move-error-banner");
    await expect(banner).toBeVisible();
    await expect(banner).toContainText("Completion requirements are not met.");

    // Reasons stay collapsed until asked for, then name the criterion and fix.
    await expect(banner.getByTestId("transition-failure-reason")).toHaveCount(0);
    await banner.getByTestId("transition-failure-details-toggle").click();
    const reason = banner.getByTestId("transition-failure-reason");
    await expect(reason).toHaveCount(1);
    await expect(reason).toHaveAttribute("data-rule", "completion_gate.criterion_unverified");
    await expect(reason).toContainText("Criterion C1");
    await expect(reason).toContainText(CRITERION_DESCRIPTION);
    await expect(reason).toContainText("Verify it with evidence.");
    await expect(banner.getByTestId("transition-failure-actionability")).toHaveText(
      "You can fix this",
    );

    // The move did not happen.
    const { tasks } = await apiClient.listTasks(seedData.workspaceId);
    expect(tasks.find((task) => task.id === taskId)?.workflow_step_id).toBe(seedData.startStepId);
  });
});
