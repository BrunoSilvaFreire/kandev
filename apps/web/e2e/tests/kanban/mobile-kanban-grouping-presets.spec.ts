// Mobile counterpart of kanban-grouping-presets.spec.ts (Pixel 5 /
// mobile-chrome). The phone board shows one workflow at a time, so the outer
// key renders as lanes inside the focused workflow; the listing menu offers
// the same grouping and preset controls, sized for touch.
import { test, expect } from "../../fixtures/test-base";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";
import {
  GROUP_NAME,
  chooseOption,
  saveNewPreset,
  seedGroupedBoard,
} from "./kanban-grouping-presets-helpers";

test.describe("Mobile Kanban grouping and presets", () => {
  test("groups the focused workflow and saves a preset from the listing menu", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await seedGroupedBoard(apiClient, seedData);
    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();

    await mobile.viewOptionsButton.tap();
    const section = testPage.getByTestId("mobile-grouping-section");
    await section.scrollIntoViewIfNeeded();
    const groupSelect = section.getByTestId("kanban-group-select");
    expect((await groupSelect.boundingBox())?.height ?? 0).toBeGreaterThanOrEqual(44);
    await chooseOption(testPage, groupSelect, "Repository group");
    expect(
      (await section.getByTestId("kanban-presets-actions").boundingBox())?.height ?? 0,
    ).toBeGreaterThanOrEqual(44);
    await saveNewPreset(testPage, section, "kanban", "Phone preset");
    await expect(section.getByTestId("kanban-presets-select")).toContainText("Phone preset");
    await testPage.keyboard.press("Escape");

    // The focused workflow's board is split into the group's lanes.
    await expect(testPage.getByTestId("kanban-group-lanes")).toBeVisible();
    await expect(testPage.getByTestId("kanban-group-lanes")).toContainText(GROUP_NAME);
    // No per-board outer sections on the phone.
    await expect(testPage.getByTestId("kanban-global-group")).toHaveCount(0);
  });
});
