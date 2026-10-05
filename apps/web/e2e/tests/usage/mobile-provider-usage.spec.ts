import { test, expect } from "../../fixtures/test-base";
import { useRegularMode } from "../../helpers/regular-mode";
import { waitForSessionDone } from "../../helpers/session";
import type { BackendContext } from "../../fixtures/backend";
import type { Page } from "@playwright/test";

// The Usage page is Office-independent; run with office off. Runs in the
// mobile-chrome project (Pixel 5) because the filename starts with "mobile-".
useRegularMode();

async function usageIndexState(backend: BackendContext, page: Page): Promise<string> {
  const response = await page.request.get(`${backend.baseUrl}/api/v1/provider-usage/index`);
  expect(response.ok()).toBe(true);
  return ((await response.json()) as { state: string }).state;
}

test.describe("Mobile provider usage page", () => {
  test("phone nav sheet reaches the page and it renders with opted-out sources", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const { agents } = await apiClient.listAgents();
    const agent = agents[0];
    expect(agent).toBeDefined();
    const mockProfile = await apiClient.createAgentProfile(agent!.id, "Usage phone mock", {
      model: "mock-fast",
      tags: ["mock-quota-55"],
    });

    try {
      await testPage.goto(`${backend.baseUrl}/?workspaceId=${seedData.workspaceId}`);
      await testPage.getByTestId("app-nav-trigger").click();
      // The desktop sidebar copies the same href but is hidden below md; the
      // nav sheet row is the visible one on a phone.
      const usageLink = testPage.locator('a[href="/usage"]:visible').first();
      await expect(usageLink).toBeVisible({ timeout: 10_000 });
      await usageLink.click();

      await testPage.setViewportSize({ width: 390, height: 700 });
      await expect(testPage.getByTestId("usage-topbar")).toContainText("Usage", {
        timeout: 15_000,
      });
      const cards = testPage.getByTestId("usage-provider-card");
      await expect(cards.first()).toBeVisible({ timeout: 15_000 });
      await expect(cards.filter({ hasText: "55%" }).first()).toBeVisible();

      // The header chrome must not clip the card stack on a short phone: the
      // last card scrolls into view inside the page scroll container.
      const lastCard = cards.last();
      await lastCard.scrollIntoViewIfNeeded();
      await expect(lastCard).toBeVisible();

      const sources = testPage.getByTestId("usage-sources-card");
      await expect(sources).toBeVisible({ timeout: 15_000 });
      await expect(sources.getByRole("switch", { name: "Kandev sessions" })).toBeChecked();
      await expect(sources.getByRole("switch", { name: "Codex history" })).not.toBeChecked();

      await expect.poll(() => usageIndexState(backend, testPage), { timeout: 30_000 }).toBe("done");
    } finally {
      await apiClient.deleteAgentProfile(mockProfile.id).catch(() => undefined);
    }
  });

  test("breakdown cards, filters sheet, and no horizontal overflow", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(240_000);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Usage mobile breakdown seed",
      seedData.agentProfileId,
      {
        description: "/background 100ms",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.worktreeExecutorProfileId,
      },
    );
    try {
      if (task.session_id) {
        await waitForSessionDone(
          apiClient,
          task.id,
          task.session_id,
          "mobile usage seed session should finish",
          90_000,
        );
      }
      await expect
        .poll(
          async () => {
            const response = await testPage.request.get(
              `${backend.baseUrl}/api/v1/provider-usage/breakdown?group_by=session&range=24h`,
            );
            return ((await response.json()) as { total_rows: number }).total_rows;
          },
          { message: "seeded usage should reach the breakdown ledger", timeout: 30_000 },
        )
        .toBeGreaterThanOrEqual(1);

      await testPage.goto(`${backend.baseUrl}/usage?tab=breakdown`);
      await expect(testPage.getByTestId("usage-breakdown")).toBeVisible({ timeout: 15_000 });

      // The desktop table is replaced by stacked cards on a phone.
      await expect(testPage.getByTestId("usage-breakdown-card").first()).toBeVisible({
        timeout: 15_000,
      });
      await expect(testPage.getByTestId("usage-breakdown-table")).toHaveCount(0);

      // Filters live behind a bottom sheet rather than an inline toolbar.
      await testPage.getByTestId("usage-breakdown-filters-button").click();
      const sheet = testPage.getByRole("dialog");
      await expect(sheet).toBeVisible();
      await expect(sheet).toContainText("Filters");
      await sheet.getByRole("button", { name: "Done" }).click();
      await expect(sheet).not.toBeVisible();

      // No horizontal document overflow on a phone.
      const overflow = await testPage.evaluate(
        () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
      );
      expect(overflow).toBeLessThanOrEqual(1);
    } finally {
      await apiClient.deleteTask(task.id).catch(() => undefined);
    }
  });
});
