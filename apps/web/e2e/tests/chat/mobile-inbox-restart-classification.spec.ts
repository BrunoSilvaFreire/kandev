// Mobile counterpart of inbox-restart-classification.spec.ts (Pixel 5 /
// mobile-chrome): the same reasons and actions, reachable by touch.
import { test, expect } from "../../fixtures/test-base";
import { assertRestartLane, seedRestartInboxTasks } from "./inbox-restart-helpers";

test.describe("Mobile inbox classification across restart", () => {
  test("keeps interrupted and review rows truthful and opens the task by tap", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(180_000);
    const { interrupted, review } = await seedRestartInboxTasks(apiClient, seedData);
    await assertRestartLane(testPage, interrupted.id, review.id);

    await backend.restart();
    await assertRestartLane(testPage, interrupted.id, review.id);

    const row = testPage.getByTestId("inbox-triage-row-interrupted");
    const box = await row.boundingBox();
    expect(box?.height ?? 0).toBeGreaterThanOrEqual(44);
    await row.tap();
    await expect(testPage).toHaveURL(new RegExp(`/t/${interrupted.id}`));
  });
});
