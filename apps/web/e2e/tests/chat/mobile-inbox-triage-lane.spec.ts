// Mobile counterpart of inbox-triage-lane.spec.ts (Pixel 5 / mobile-chrome).
// Asserts the same ordered lane and acknowledgement behavior, and that the
// nudge is visible from the phone app-navigation sheet. The sheet omits the
// Inbox entry while the Inbox route is open, so the badge is read from Home.
import { test, expect } from "../../fixtures/test-base";
import { assertTriageLane, seedTriageTasks } from "./inbox-triage-helpers";

test.describe("Mobile Inbox Triage lane", () => {
  test("surfaces the nudge, orders the lane, clears it, and opens the conversation", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const { question } = await seedTriageTasks(apiClient, seedData);

    await testPage.goto("/");
    await testPage.getByTestId("app-nav-trigger").tap();
    const sheetInboxLink = testPage
      .getByTestId("app-nav-sheet")
      .getByRole("link", { name: /Inbox/ });
    await expect(sheetInboxLink).toContainText("2", { timeout: 30_000 });
    await testPage.keyboard.press("Escape");

    await testPage.goto("/needs-you-inbox");
    const questionRow = await assertTriageLane(testPage, question.id);
    await expect(testPage.getByTestId("inbox-tab-triage-badge")).toHaveCount(0, {
      timeout: 15_000,
    });

    await testPage.goto("/");
    await testPage.getByTestId("app-nav-trigger").tap();
    const clearedSheetLink = testPage
      .getByTestId("app-nav-sheet")
      .getByRole("link", { name: /Inbox/ });
    await expect(clearedSheetLink).toContainText("1");
    await expect(clearedSheetLink).not.toContainText("2");
    await testPage.keyboard.press("Escape");

    await testPage.goto("/needs-you-inbox");
    await testPage.getByTestId("inbox-tab-triage").click();
    await questionRow.click();
    await expect(testPage).toHaveURL(new RegExp(`/t/${question.id}`));
  });
});
