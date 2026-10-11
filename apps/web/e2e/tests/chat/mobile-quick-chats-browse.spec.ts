// Phone sibling of quick-chats-browse.spec.ts: the same order control, dates,
// titles, and badges, sized for touch.
import { test, expect } from "../../fixtures/test-base";
import { CREATED_TASK_TITLE, FIRST_QUESTION, seedBrowseChats } from "./quick-chats-browse-helpers";

test.describe("Mobile Quick Chats browse", () => {
  test("shows activity order, titles, and tappable links", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const { named, unnamed, createdTaskId } = await seedBrowseChats(apiClient, seedData);

    await testPage.goto("/quick-chats");
    const unnamedRow = testPage.getByTestId(`quick-chat-row-${unnamed.session_id}`);
    await expect(unnamedRow).toBeVisible({ timeout: 15_000 });
    await expect(unnamedRow.getByTestId("quick-chat-row-title")).toHaveText(FIRST_QUESTION);
    await expect(testPage.getByTestId(`quick-chat-activity-${named.session_id}`)).toContainText(
      "Sep 2, 2026",
    );
    await expect(
      testPage.getByTestId(`quick-chat-repository-badge-${seedData.repositoryId}`),
    ).toBeVisible();

    const order = testPage.getByTestId("quick-chats-order");
    expect((await order.boundingBox())?.height ?? 0).toBeGreaterThanOrEqual(44);
    await order.tap();
    await testPage.getByRole("option", { name: "Oldest activity first" }).tap();
    const firstRow = testPage
      .getByTestId("quick-chats-list")
      .getByTestId("quick-chat-browse-row")
      .first();
    await expect(firstRow.getByTestId(`quick-chat-row-${named.session_id}`)).toBeVisible();

    // No horizontal overflow from dates or badges.
    const overflow = await testPage.evaluate(
      () => document.documentElement.scrollWidth - window.innerWidth,
    );
    expect(overflow).toBeLessThanOrEqual(1);

    const taskBadge = testPage.getByTestId(`quick-chat-task-badge-${createdTaskId}`);
    await expect(taskBadge).toContainText(CREATED_TASK_TITLE);
    expect((await taskBadge.boundingBox())?.height ?? 0).toBeGreaterThanOrEqual(44);
    await taskBadge.tap();
    await expect(testPage).toHaveURL(new RegExp(`/t/${createdTaskId}`));
  });
});
