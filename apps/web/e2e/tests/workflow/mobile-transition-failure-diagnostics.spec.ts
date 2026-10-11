// Phone sibling of transition-failure-diagnostics.spec.ts: the next-step
// button only has a toast, so the refusal stays in the app-wide notice and its
// reasons open in a touch drawer.
import { test, expect } from "../../fixtures/test-base";
import {
  CRITERION_DESCRIPTION,
  doneStep,
  seedGateBlockedTask,
} from "./transition-failure-diagnostics-helpers";

test.describe("Mobile transition failure diagnostics", () => {
  test("keeps the refusal reasons reachable in a touch drawer", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const done = doneStep(seedData);
    const beforeDone = [...seedData.steps]
      .filter((step) => step.position < done.position)
      .sort((a, b) => b.position - a.position)[0];
    if (!beforeDone) throw new Error("seed workflow has no step before Done");
    const taskId = await seedGateBlockedTask(apiClient, seedData, beforeDone.id);

    await testPage.goto(`/t/${taskId}`);
    const nextStep = testPage.getByTestId("proceed-next-step");
    await expect(nextStep).toBeVisible({ timeout: 15_000 });
    await nextStep.tap();

    const notice = testPage.getByTestId("transition-failure-notice");
    await expect(notice).toBeVisible();
    await expect(notice).toContainText("Completion requirements are not met.");

    const toggle = notice.getByTestId("transition-failure-details-toggle");
    expect((await toggle.boundingBox())?.height ?? 0).toBeGreaterThanOrEqual(44);
    await toggle.tap();
    const drawer = testPage.getByTestId("transition-failure-drawer");
    await expect(drawer).toBeVisible();
    const reason = drawer.getByTestId("transition-failure-reason");
    await expect(reason).toContainText("Criterion C1");
    await expect(reason).toContainText(CRITERION_DESCRIPTION);
    await expect(reason).toContainText("Verify it with evidence.");
    await testPage.keyboard.press("Escape");

    // The task stayed put, and the notice goes away only when dismissed.
    expect((await apiClient.getTask(taskId)).workflow_step_id).toBe(beforeDone.id);
    await notice.getByTestId("transition-failure-notice-dismiss").tap();
    await expect(notice).toHaveCount(0);
  });
});
