// This file starts with `mobile-` so Playwright runs it on the Pixel 5 project.
import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

const WORKFLOW_NAME = "Mobile transition summary workflow";
const WORKFLOW_YAML = `version: 1
type: kandev_workflow
workflows:
  - name: ${WORKFLOW_NAME}
    steps:
      - name: Backlog
        position: 0
        color: bg-neutral-400
        events:
          on_turn_complete:
            - type: move_to_next
      - name: Implementation
        position: 1
        color: bg-blue-500
        events:
          on_turn_complete:
            - type: move_to_next
      - name: Review
        position: 2
        color: bg-yellow-500
        is_start_step: true
        allow_manual_move: true`;

test.describe("Mobile task topbar workflow transitions", () => {
  test("opens the transition summary in the touch drawer", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const imported = await apiClient.importWorkflows(seedData.workspaceId, WORKFLOW_YAML);
    expect(imported.created).toContain(WORKFLOW_NAME);
    const { workflows } = await apiClient.listWorkflows(seedData.workspaceId);
    const workflow = workflows.find(({ name }) => name === WORKFLOW_NAME);
    if (!workflow) throw new Error("mobile transition summary workflow was not imported");
    const { steps } = await apiClient.listWorkflowSteps(workflow.id);
    const review = steps.find(({ name }) => name === "Review");
    if (!review) throw new Error("mobile transition summary workflow is missing Review");

    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: workflow.id,
    });
    const task = await apiClient.seedTask(seedData.workspaceId, "Mobile transition summary task", {
      workflow_id: workflow.id,
      workflow_step_id: review.id,
      state: "REVIEW",
    });

    await testPage.goto(`/t/${task.task_id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const expand = testPage.getByTestId("workflow-stepper-expand");
    await expect(expand).toBeVisible({ timeout: 15_000 });
    // Coarse pointer: the control must meet the 44px touch minimum.
    expect((await expand.boundingBox())?.height ?? 0).toBeGreaterThanOrEqual(44);
    await expand.tap();

    const drawer = testPage.getByTestId("workflow-transition-drawer");
    await expect(drawer).toBeVisible();
    await expect(drawer.getByTestId("workflow-transition-edge").first()).toBeVisible();

    const box = await drawer.boundingBox();
    const viewportHeight = await testPage.evaluate(() => window.innerHeight);
    expect(box?.height ?? 0).toBeLessThanOrEqual(viewportHeight + 1);
  });
});
