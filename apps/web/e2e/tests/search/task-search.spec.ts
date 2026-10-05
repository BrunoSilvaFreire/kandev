// Task-wide message search from the existing Ctrl/Cmd+F experience.
import { test, expect } from "../../fixtures/test-base";
import { openPanelSearch, panelSearchInput } from "../../helpers/panel-search";
import { seedTask, seedMessagesDescription } from "./shared";

const HITS_LIST_SELECTOR = "[data-panel-search-bar] + div";

test.describe("@search task-wide message search", () => {
  test("C1 searches the whole task and focuses the chosen hit", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const marker = "taskwide-sentinel-7q2";
    const { session } = await seedTask(testPage, apiClient, seedData, "task-search-wide", {
      description: seedMessagesDescription([`alpha line with ${marker}`, "beta line plain"]),
    });
    await expect(session.chat.getByText(marker, { exact: false }).first()).toBeVisible({
      timeout: 30_000,
    });

    await openPanelSearch(testPage, "session");
    await panelSearchInput(testPage).fill(marker);

    const hits = testPage.locator(HITS_LIST_SELECTOR);
    await expect(hits).toBeVisible({ timeout: 10_000 });
    await expect(hits.locator("mark").first()).toBeVisible({ timeout: 5_000 });
    expect(await hits.locator("button").count()).toBeGreaterThanOrEqual(1);

    await hits.locator("button").first().click();
    await expect
      .poll(
        async () => testPage.evaluate(() => document.querySelectorAll(".search-flash").length > 0),
        { timeout: 5_000, message: "Expected .search-flash on the focused task-search hit" },
      )
      .toBe(true);
  });

  test("C2 unmatched query reports no matches without leaving the task", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const { session } = await seedTask(testPage, apiClient, seedData, "task-search-nomatch", {
      description: seedMessagesDescription(["task search content only"]),
    });
    await expect(
      session.chat.getByText("task search content", { exact: false }).first(),
    ).toBeVisible({ timeout: 30_000 });

    await openPanelSearch(testPage, "session");
    await panelSearchInput(testPage).fill("zzznomatchzzz");
    await expect(testPage.getByText("No matches")).toBeVisible({ timeout: 10_000 });
  });
});
