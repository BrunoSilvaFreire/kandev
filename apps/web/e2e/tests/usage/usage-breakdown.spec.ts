import { test, expect } from "../../fixtures/test-base";
import { useRegularMode } from "../../helpers/regular-mode";
import { waitForSessionDone } from "../../helpers/session";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import type { BackendContext } from "../../fixtures/backend";
import type { Page } from "@playwright/test";

// The Usage page is Office-independent; run with office off for a plain shell.
useRegularMode();

/**
 * Seeds one real usage row through the ledger writer. `/background 100ms` makes
 * the mock agent emit a context-window usage boundary, which the production
 * writer persists to task_usage_events; no test-only seed route is used.
 */
async function seedUsageTask(
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
): Promise<{ id: string }> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: "/background 100ms",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
      executor_profile_id: seedData.worktreeExecutorProfileId,
    },
  );
  if (task.session_id) {
    await waitForSessionDone(
      apiClient,
      task.id,
      task.session_id,
      "usage seed session should finish",
      90_000,
    );
  }
  return { id: task.id };
}

async function breakdownTotalRows(
  backend: BackendContext,
  page: Page,
  query = "group_by=session&range=24h",
): Promise<number> {
  const response = await page.request.get(
    `${backend.baseUrl}/api/v1/provider-usage/breakdown?${query}`,
  );
  expect(response.ok()).toBe(true);
  return ((await response.json()) as { total_rows: number }).total_rows;
}

test.describe("Usage breakdown", () => {
  test("groups, searches, sorts, and drills into seeded usage", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(240_000);
    const created: string[] = [];
    try {
      created.push((await seedUsageTask(apiClient, seedData, "Usage breakdown seed A")).id);
      created.push((await seedUsageTask(apiClient, seedData, "Usage breakdown seed B")).id);

      await expect
        .poll(() => breakdownTotalRows(backend, testPage), {
          message: "seeded usage should reach the breakdown ledger",
          timeout: 30_000,
        })
        .toBeGreaterThanOrEqual(2);

      await testPage.goto(`${backend.baseUrl}/usage?tab=breakdown`);
      await expect(testPage.getByTestId("usage-breakdown")).toBeVisible({ timeout: 15_000 });

      const rows = testPage.getByTestId("usage-breakdown-row");
      await expect(rows.first()).toBeVisible({ timeout: 15_000 });

      // Search narrows the session view to the matching task.
      const search = testPage.getByTestId("usage-breakdown-search");
      await search.fill("Usage breakdown seed A");
      await expect(rows).toHaveCount(1, { timeout: 15_000 });
      await expect(rows.first()).toContainText("Usage breakdown seed A");

      // Clear the search through its removable chip.
      await testPage.getByRole("button", { name: "Remove Search filter" }).click();
      await expect(rows).toHaveCount(2, { timeout: 15_000 });

      // Sort header reports and toggles its direction.
      const tokensHeader = testPage.getByRole("columnheader", { name: "Tokens", exact: true });
      await expect(tokensHeader).toHaveAttribute("aria-sort", "descending");
      await tokensHeader.getByRole("button").click();
      await expect(tokensHeader).toHaveAttribute("aria-sort", "ascending");

      // A row click drills into its models and adds a filter chip.
      await rows.first().click();
      const filterChips = testPage.getByTestId("usage-breakdown-filters");
      await expect(filterChips).toBeVisible();
      await expect(filterChips).toContainText("Session");

      // Removing the chip restores the full session rows.
      await filterChips.getByRole("button", { name: /Remove Session filter/ }).click();
      await expect(filterChips).not.toBeVisible();
      await testPage
        .getByTestId("usage-breakdown-groups")
        .getByRole("radio", { name: "Session" })
        .click();
      await expect(rows).toHaveCount(2, { timeout: 15_000 });
    } finally {
      for (const taskId of created) {
        await apiClient.deleteTask(taskId).catch(() => undefined);
      }
    }
  });
});
