import { expect, type Locator, type Page } from "@playwright/test";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";

export const GROUP_NAME = "Alpha Group";
export const SECOND_WORKFLOW = "Grouping second";

/**
 * Seeds two workflows that both hold a task in one repository group, plus one
 * task outside every group, and shows every workflow on the board.
 */
export async function seedGroupedBoard(api: ApiClient, seed: SeedData) {
  const workflow = await api.createWorkflow(seed.workspaceId, SECOND_WORKFLOW, "simple");
  const { steps } = await api.listWorkflowSteps(workflow.id);
  const step = steps.find((item) => item.is_start_step) ?? steps[0];
  await api.createRepositorySet(seed.workspaceId, GROUP_NAME, [seed.repositoryId]);
  const inGroupFirst = await api.createTask(seed.workspaceId, "Grouped in first workflow", {
    workflow_id: seed.workflowId,
    workflow_step_id: seed.startStepId,
    repository_ids: [seed.repositoryId],
  });
  const inGroupSecond = await api.createTask(seed.workspaceId, "Grouped in second workflow", {
    workflow_id: workflow.id,
    workflow_step_id: step.id,
    repository_ids: [seed.repositoryId],
  });
  const ungrouped = await api.createTask(seed.workspaceId, "No repository task", {
    workflow_id: seed.workflowId,
    workflow_step_id: seed.startStepId,
  });
  await api.saveUserSettings({ workflow_filter_id: "", repository_ids: [] });
  return { workflow, inGroupFirst, inGroupSecond, ungrouped };
}

/** Picks an option in a shared Select by its visible name. */
export async function chooseOption(page: Page, trigger: Locator, name: string) {
  await trigger.click();
  await page.getByRole("option", { name, exact: true }).click();
}

/**
 * Saves the live filters and grouping as a new preset through the dialog.
 * `scope` is the surface holding the preset control; the menu and dialog are
 * portalled to the page.
 */
export async function saveNewPreset(
  page: Page,
  scope: Page | Locator,
  view: "kanban" | "list",
  name: string,
) {
  await scope.getByTestId(`${view}-presets-actions`).click();
  await page.getByTestId(`${view}-presets-save-as`).click();
  await page.getByTestId("view-preset-name-input").fill(name);
  await page.getByTestId("view-preset-name-save").click();
  await expect(page.getByTestId("view-preset-name-dialog")).toHaveCount(0);
}
