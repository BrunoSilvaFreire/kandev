import { test, expect } from "../../fixtures/test-base";

/**
 * Quick Chats browse surface: the sidebar entry opens a page that lists the
 * workspace's Quick Chats and opens one into the shared detail page.
 */
test.describe("Quick Chats surface", () => {
  test("browses Quick Chats and opens a detail page", async ({ testPage, apiClient, seedData }) => {
    test.setTimeout(120_000);
    const chat = await apiClient.startQuickChat(
      seedData.workspaceId,
      seedData.agentProfileId,
      "Browse flow chat",
    );

    await testPage.goto("/quick-chats");
    await expect(testPage.getByTestId("quick-chats-topbar")).toBeVisible({ timeout: 10_000 });

    const row = testPage.getByTestId(`quick-chat-row-${chat.session_id}`);
    await expect(row).toBeVisible({ timeout: 15_000 });
    await expect(row).toContainText("Browse flow chat");

    await row.click();

    await expect(testPage).toHaveURL(new RegExp(`/quick-chats/${chat.task_id}`), {
      timeout: 15_000,
    });
    // The detail page reuses the task workbench chrome.
    await expect(testPage.getByTestId("task-topbar")).toBeVisible({ timeout: 15_000 });
    // Task-only workflow chrome is not rendered on a Quick Chat surface.
    await expect(testPage.getByTestId("workflow-stepper")).toHaveCount(0);
  });

  test("opens the browse page from the sidebar entry", async ({ testPage }) => {
    test.setTimeout(60_000);
    await testPage.goto("/");
    await testPage.waitForLoadState("networkidle");

    await testPage.getByTestId("sidebar-quick-chats").click();

    await expect(testPage).toHaveURL(/\/quick-chats$/, { timeout: 10_000 });
    await expect(testPage.getByTestId("quick-chats-topbar")).toBeVisible({ timeout: 10_000 });
  });
});
