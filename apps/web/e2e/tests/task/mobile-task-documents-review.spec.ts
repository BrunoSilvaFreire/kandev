// This file starts with `mobile-` so Playwright runs it on the Pixel 5 project.
// Phone flow: Documents opens from the existing Panels picker as a full-height
// list, and a selected document becomes a one-scroll preview with Back.
import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

const DOC_KEY = "spike";
const DOC_TITLE = "spike-report.md";
const DOC_BODY = "Mobile spike preview sentinel body.";

test.describe("Mobile task documents and Review", () => {
  test("opens Documents from Panels and previews a document with Back", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile task documents review",
      seedData.agentProfileId,
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await apiClient.writeTaskDocument(task.id, DOC_KEY, {
      type: "SPIKE",
      title: DOC_TITLE,
      content: `# Spike\n\n${DOC_BODY}`,
    });

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const panelsButton = testPage.getByRole("button", { name: "Panels", exact: true });
    await expect(panelsButton).toBeVisible({ timeout: 15_000 });
    await panelsButton.tap();

    const documentsOption = testPage.getByTestId("mobile-documents-option");
    await expect(documentsOption).toBeVisible({ timeout: 10_000 });
    expect((await documentsOption.boundingBox())?.height).toBeGreaterThanOrEqual(44);
    await documentsOption.tap();

    const list = testPage.getByTestId("mobile-documents-list");
    await expect(list).toBeVisible({ timeout: 10_000 });
    const row = testPage.getByTestId(`document-row-${DOC_KEY}`);
    await expect(row).toContainText(DOC_TITLE);
    expect((await row.boundingBox())?.height).toBeGreaterThanOrEqual(44);
    await row.tap();

    const preview = testPage.getByTestId("mobile-documents-preview");
    await expect(preview).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByTestId("task-document-review-body")).toContainText(DOC_BODY);

    const back = testPage.getByTestId("mobile-documents-back");
    await expect(back).toBeVisible();
    expect((await back.boundingBox())?.height).toBeGreaterThanOrEqual(44);
    await back.tap();

    await expect(list).toBeVisible();
    await expect(row).toContainText(DOC_TITLE);
  });
});
