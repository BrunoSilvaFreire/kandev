import { test, expect } from "../../fixtures/test-base";
import { useRegularMode } from "../../helpers/regular-mode";
import type { BackendContext } from "../../fixtures/backend";
import type { Page } from "@playwright/test";

// The Usage page is Office-independent; run with office off for a plain shell.
useRegularMode();

async function usageIndexState(backend: BackendContext, page: Page): Promise<string> {
  const response = await page.request.get(`${backend.baseUrl}/api/v1/provider-usage/index`);
  expect(response.ok()).toBe(true);
  return ((await response.json()) as { state: string }).state;
}

test.describe("Provider usage page", () => {
  test("sidebar order, provider cards, opt-in sources, and index completion", async ({
    testPage,
    apiClient,
    backend,
  }) => {
    test.setTimeout(120_000);
    const { agents } = await apiClient.listAgents();
    const agent = agents[0];
    expect(agent).toBeDefined();
    // mock-quota tags are read only under KANDEV_E2E_MOCK (the e2e profile).
    const mockProfile = await apiClient.createAgentProfile(agent!.id, "Usage mock account", {
      model: "mock-fast",
      tags: ["mock-quota-42"],
    });

    try {
      await testPage.goto(`${backend.baseUrl}/usage`);

      await expect(testPage.getByTestId("usage-topbar")).toContainText("Usage");

      // The sidebar item sits between Inbox and New Task.
      const usageItem = testPage.getByTestId("sidebar-usage").first();
      await expect(usageItem).toBeVisible();
      const usageBox = await usageItem.boundingBox();
      const newTaskBox = await testPage.getByTestId("create-task-button").first().boundingBox();
      expect(usageBox?.y ?? 0).toBeLessThan(newTaskBox?.y ?? 0);
      const inbox = testPage.getByTestId("sidebar-needs-you-inbox").first();
      if ((await inbox.count()) > 0) {
        const inboxBox = await inbox.boundingBox();
        expect(inboxBox?.y ?? 0).toBeLessThan(usageBox?.y ?? 0);
      }

      // Provider cards render and the mock account shows its measured window.
      const cards = testPage.getByTestId("usage-provider-card");
      await expect(cards.first()).toBeVisible({ timeout: 15_000 });
      await expect(testPage.getByTestId("usage-window").first()).toBeVisible({ timeout: 15_000 });
      await expect(cards.filter({ hasText: "42%" }).first()).toBeVisible();

      // The page owns a scroll container below the topbar: the last card must
      // be reachable even when the card stack overflows.
      const lastCard = cards.last();
      await lastCard.scrollIntoViewIfNeeded();
      await expect(lastCard).toBeVisible();

      // Window metadata renders as chips; hovering one explains it.
      const estimateChip = testPage.getByTestId("usage-estimate-chip-notEnough").first();
      await expect(estimateChip).toBeVisible();
      await estimateChip.hover();
      await expect(testPage.getByRole("tooltip")).toContainText("needs more samples");
      const resetChip = testPage.getByTestId("usage-reset-chip").first();
      await expect(resetChip).toBeVisible();

      // History sources: Kandev is always included, local sources default off.
      const sources = testPage.getByTestId("usage-sources-card");
      await expect(sources).toBeVisible({ timeout: 15_000 });
      const kandev = sources.getByRole("switch", { name: "Kandev sessions" });
      await expect(kandev).toBeChecked();
      await expect(kandev).toBeDisabled();
      for (const name of ["Claude Code history", "Codex history", "Antigravity CLI logs"]) {
        await expect(sources.getByRole("switch", { name })).not.toBeChecked();
      }

      // The Kandev-only index (empty temp HOME) reaches done.
      await expect.poll(() => usageIndexState(backend, testPage), { timeout: 30_000 }).toBe("done");
    } finally {
      await apiClient.deleteAgentProfile(mockProfile.id).catch(() => undefined);
    }
  });
});
