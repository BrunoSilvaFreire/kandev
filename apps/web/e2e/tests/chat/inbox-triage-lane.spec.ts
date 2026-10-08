// The Inbox Triage lane is a sibling of the clarification-backed Needs-you
// tab: it renders real clarifications first, then stale/interrupted Review
// tasks, then advisory possible-question hints. This covers the lane's real
// contents, ordering, the surfaced nudge, acknowledgement clearing it, and
// navigation to the conversation on desktop. The mobile-chrome sibling lives
// in mobile-inbox-triage-lane.spec.ts.
import { test, expect } from "../../fixtures/test-base";
import { assertTriageLane, seedTriageTasks } from "./inbox-triage-helpers";

test.describe("Inbox Triage lane", () => {
  test("orders real items, surfaces the nudge, clears it, and opens the conversation", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const { question } = await seedTriageTasks(apiClient, seedData);

    await testPage.goto("/needs-you-inbox");

    // The clarification-backed count (1) plus the lane-only possible-question
    // nudge (1) land on the navigation entry before the lane is opened.
    const sidebarInbox = testPage.getByTestId("sidebar-needs-you-inbox");
    await expect(sidebarInbox).toContainText("2", { timeout: 30_000 });

    const questionRow = await assertTriageLane(testPage, question.id);

    // Viewing the lane acknowledges the surfaced identities: the lane badge
    // clears and the navigation entry drops back to the clarification count.
    await expect(testPage.getByTestId("inbox-tab-triage-badge")).toHaveCount(0, {
      timeout: 15_000,
    });
    await expect(sidebarInbox).toContainText("1");
    await expect(sidebarInbox).not.toContainText("2");

    await questionRow.click();
    await expect(testPage).toHaveURL(new RegExp(`/t/${question.id}`));
  });
});
