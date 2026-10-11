// Restart recovery must not manufacture review requests or duplicate inbox
// rows: an interrupted task stays resumable work, a genuine review stays a
// review, and two consecutive restarts leave exactly one row per task. The
// mobile-chrome sibling lives in mobile-inbox-restart-classification.spec.ts.
import { test, expect } from "../../fixtures/test-base";
import { assertRestartLane, seedRestartInboxTasks } from "./inbox-restart-helpers";

test.describe("Inbox classification across restart", () => {
  test("keeps interrupted and review rows truthful and unduplicated", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(180_000);
    const { interrupted, review } = await seedRestartInboxTasks(apiClient, seedData);
    await assertRestartLane(testPage, interrupted.id, review.id);

    for (let round = 0; round < 2; round += 1) {
      await backend.restart();
      await assertRestartLane(testPage, interrupted.id, review.id);
    }

    // The restart never moved the genuine review or re-stamped its entry.
    const after = await apiClient.getTask(review.id);
    expect(after.state).toBe("REVIEW");
  });
});
