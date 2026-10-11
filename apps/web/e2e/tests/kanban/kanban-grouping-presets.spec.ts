// Nested grouping and named presets on the desktop board: a repository group
// is shown once across workflows with its workflows nested beneath, and a
// saved preset restores filters and grouping after a reload and a restart.
// The mobile-chrome sibling lives in mobile-kanban-grouping-presets.spec.ts.
import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import {
  GROUP_NAME,
  SECOND_WORKFLOW,
  chooseOption,
  saveNewPreset,
  seedGroupedBoard,
} from "./kanban-grouping-presets-helpers";

test.describe("Kanban nested grouping and presets", () => {
  test("groups globally, nests workflows, and persists a named preset", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(180_000);
    const { inGroupFirst, inGroupSecond, ungrouped } = await seedGroupedBoard(apiClient, seedData);
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await expect(kanban.taskCard(inGroupFirst.id)).toBeVisible();

    // Repository group only: each group heading appears exactly once, even
    // though two workflows hold tasks in the same group.
    await chooseOption(testPage, testPage.getByTestId("kanban-group-select"), "Repository group");
    const groups = testPage.getByTestId("kanban-global-group");
    await expect(groups).toHaveCount(2);
    const headers = testPage.getByTestId("kanban-global-group-header");
    await expect(headers.filter({ hasText: GROUP_NAME })).toHaveCount(1);

    // Both workflows are nested under the one group; the ungrouped task has
    // its own single group that only the first workflow appears in.
    const alpha = groups.filter({ hasText: GROUP_NAME });
    await expect(alpha.getByTestId("swimlane-header")).toHaveCount(2);
    await expect(
      alpha.getByTestId("swimlane-header").filter({ hasText: SECOND_WORKFLOW }),
    ).toHaveCount(1);
    await expect(alpha.getByTestId(`task-card-${inGroupFirst.id}`)).toBeVisible();
    await expect(alpha.getByTestId(`task-card-${inGroupSecond.id}`)).toBeVisible();
    await expect(alpha.getByTestId(`task-card-${ungrouped.id}`)).toHaveCount(0);
    const rest = groups.filter({ hasNot: testPage.getByText(GROUP_NAME, { exact: true }) });
    await expect(rest.getByTestId(`task-card-${ungrouped.id}`)).toBeVisible();
    await expect(rest.getByTestId("swimlane-header")).toHaveCount(1);

    // Repository group then workflow keeps the same nesting.
    await chooseOption(testPage, testPage.getByTestId("kanban-group-then-select"), "Workflow");
    await expect(groups).toHaveCount(2);
    await expect(alpha.getByTestId("swimlane-header")).toHaveCount(2);

    // Save it, drift from it, and get back to it by applying the preset.
    await saveNewPreset(testPage, testPage, "kanban", "By group");
    await expect(testPage.getByTestId("kanban-presets-select")).toContainText("By group");
    await expect(testPage.getByTestId("kanban-presets-dirty")).toHaveCount(0);

    await chooseOption(testPage, testPage.getByTestId("kanban-group-select"), "None");
    await expect(groups).toHaveCount(0);
    await expect(testPage.getByTestId("kanban-presets-dirty")).toBeVisible();

    await chooseOption(testPage, testPage.getByTestId("kanban-presets-select"), "By group");
    await expect(groups).toHaveCount(2);
    await expect(testPage.getByTestId("kanban-presets-dirty")).toHaveCount(0);

    // A second name that only differs by case is refused with a reason.
    await testPage.getByTestId("kanban-presets-actions").click();
    await testPage.getByTestId("kanban-presets-save-as").click();
    await testPage.getByTestId("view-preset-name-input").fill("by GROUP");
    await testPage.getByTestId("view-preset-name-save").click();
    await expect(testPage.getByTestId("view-preset-name-error")).toBeVisible();
    await testPage.keyboard.press("Escape");

    // The preset, the active marker, and the grouping survive a reload and a
    // backend restart.
    await testPage.reload();
    await expect(groups).toHaveCount(2);
    await expect(testPage.getByTestId("kanban-presets-select")).toContainText("By group");
    await backend.restart();
    await testPage.reload();
    await expect(groups).toHaveCount(2);
    await expect(testPage.getByTestId("kanban-presets-select")).toContainText("By group");

    // Deleting the active preset keeps the live grouping and clears the marker.
    await testPage.getByTestId("kanban-presets-actions").click();
    await testPage.getByTestId("kanban-presets-delete").click();
    await expect(testPage.getByTestId("kanban-presets-select")).not.toContainText("By group");
    await expect(groups).toHaveCount(2);
  });
});
