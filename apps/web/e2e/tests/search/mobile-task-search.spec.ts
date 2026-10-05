// This file starts with `mobile-` so Playwright runs it on the Pixel 5 project.
// Phone task search must open a full-height surface and land on the hit.
import { test, expect } from "../../fixtures/test-base";
import { openPanelSearch } from "../../helpers/panel-search";
import { seedTask, seedMessagesDescription } from "./shared";

test.describe("@search mobile task-wide message search", () => {
  test("opens a full-height search surface and focuses the chosen hit", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const marker = "mobile-task-sentinel-4k1";
    const { session } = await seedTask(testPage, apiClient, seedData, "mobile-task-search", {
      description: seedMessagesDescription([`gamma line with ${marker}`]),
    });
    await expect(session.chat.getByText(marker, { exact: false }).first()).toBeVisible({
      timeout: 30_000,
    });

    await openPanelSearch(testPage, "session");
    const surface = testPage.getByTestId("task-search-surface");
    await expect(surface).toBeVisible({ timeout: 5_000 });
    const box = await surface.boundingBox();
    expect(box?.height ?? 0).toBeGreaterThan(600);
    expect(box?.width ?? 0).toBeLessThanOrEqual(400);

    await surface.locator('input[type="text"]').fill(marker);
    await expect(surface.locator("mark").first()).toBeVisible({ timeout: 10_000 });
    const hit = surface.getByTestId("session-search-hits").locator("button").first();
    expect((await hit.boundingBox())?.height ?? 0).toBeGreaterThanOrEqual(44);
    await hit.tap();

    await expect
      .poll(
        async () => testPage.evaluate(() => document.querySelectorAll(".search-flash").length > 0),
        { timeout: 5_000, message: "Expected .search-flash on the focused mobile task-search hit" },
      )
      .toBe(true);
  });
});
