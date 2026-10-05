// Desktop flow: discover a task document in the metadata-only catalog and open
// the read-only Review surface on the shared Plan singleton.
import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

const DOC_KEY = "architecture";
const DOC_TITLE = "architecture.md";
const DOC_BODY = "Review body sentinel for the architecture document.";

test.describe("Task documents and Review", () => {
  test("opens a catalog document in the read-only Review surface", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Task documents review",
      seedData.agentProfileId,
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await apiClient.writeTaskDocument(task.id, DOC_KEY, {
      type: "SPEC",
      title: DOC_TITLE,
      content: `# Architecture\n\n${DOC_BODY}`,
    });

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForDockviewReady();

    await session.addPanelButton().click();
    await testPage.getByTestId("add-panel-documents-item").click();

    const documents = testPage.getByTestId("documents-panel");
    await expect(documents).toBeVisible();
    await expect(documents.getByTestId(`document-row-${DOC_KEY}`)).toContainText(DOC_TITLE);

    await documents.getByTestId(`document-row-${DOC_KEY}`).click();

    const review = testPage.getByTestId("task-document-review");
    await expect(review).toBeVisible();
    await expect(testPage.getByTestId("task-document-review-body")).toContainText(DOC_BODY);

    // Legacy/unknown provenance is explicit, never inferred from the task.
    await expect(testPage.getByTestId("document-source-label")).toHaveText("Unknown source");

    // The Review surface is read-only: no plan save control appears inside it.
    await expect(review.getByRole("button", { name: "Save" })).toHaveCount(0);
  });

  test("selects document text into a pending chat comment chip", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Task document selection",
      seedData.agentProfileId,
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await apiClient.writeTaskDocument(task.id, DOC_KEY, {
      type: "SPEC",
      title: DOC_TITLE,
      content: `# Architecture\n\n${DOC_BODY}`,
    });

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForDockviewReady();

    await session.addPanelButton().click();
    await testPage.getByTestId("add-panel-documents-item").click();
    const documents = testPage.getByTestId("documents-panel");
    await expect(documents).toBeVisible();
    await documents.getByTestId(`document-row-${DOC_KEY}`).click();

    const body = testPage.getByTestId("task-document-review-body");
    await expect(body).toContainText(DOC_BODY);
    await body.selectText();
    await body.dispatchEvent("mouseup");

    const composer = testPage.getByTestId("document-selection-popover");
    await expect(composer).toBeVisible();
    await testPage.getByTestId("document-selection-input").fill("confirm the retry limit");
    await testPage.getByTestId("document-selection-add").click();

    // The selection becomes a pending chip in the chat composer, ready to send.
    await session.showSessionContext();
    await expect(testPage.getByText("Document comment (1)")).toBeVisible();
  });
});
