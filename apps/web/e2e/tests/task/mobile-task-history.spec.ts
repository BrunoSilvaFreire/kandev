// This file starts with `mobile-` so Playwright runs it on the Pixel 5 project.
import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

const WORKFLOW_NAME = "Mobile task history workflow";
const WORKFLOW_YAML = `version: 1
type: kandev_workflow
workflows:
  - name: ${WORKFLOW_NAME}
    steps:
      - name: Plan
        position: 0
        color: bg-purple-500
        is_start_step: true
        allow_manual_move: true
      - name: Implement
        position: 1
        color: bg-blue-500
        allow_manual_move: true`;

/** Reads the exposed store so the phone activation can be asserted directly. */
async function readActiveSessionId(page: import("@playwright/test").Page): Promise<string | null> {
  return page.evaluate(() => {
    const store = (
      window as unknown as {
        __KANDEV_E2E_STORE__?: { getState: () => Record<string, unknown> };
      }
    ).__KANDEV_E2E_STORE__;
    const tasks = store?.getState()?.tasks as Record<string, unknown> | undefined;
    return (tasks?.activeSessionId as string | undefined) ?? null;
  });
}

test.describe("Mobile Task History", () => {
  test("opens the timeline from Panels and filters it from a step visit", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const imported = await apiClient.importWorkflows(seedData.workspaceId, WORKFLOW_YAML);
    expect(imported.created).toContain(WORKFLOW_NAME);
    const { workflows } = await apiClient.listWorkflows(seedData.workspaceId);
    const workflow = workflows.find(({ name }) => name === WORKFLOW_NAME);
    if (!workflow) throw new Error("mobile task history workflow was not imported");
    const { steps } = await apiClient.listWorkflowSteps(workflow.id);
    const plan = steps.find(({ name }) => name === "Plan");
    const implement = steps.find(({ name }) => name === "Implement");
    if (!plan || !implement) throw new Error("mobile task history workflow is missing steps");

    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: workflow.id,
    });
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile task history task",
      seedData.agentProfileId,
      {
        workflow_id: workflow.id,
        workflow_step_id: plan.id,
        repository_ids: [seedData.repositoryId],
      },
    );
    // Two Plan visits, plus a durable route whose Open targets the destination.
    for (const stepId of [implement.id, plan.id, implement.id, plan.id]) {
      await apiClient.moveTask(task.id, workflow.id, stepId);
    }
    // A registered destination session; Open must make it the active session.
    const destinationSessionId = "sess-mobile-history-dest";
    await apiClient.seedTaskSession(task.id, {
      state: "IDLE",
      sessionId: destinationSessionId,
      agentProfileId: seedData.agentProfileId,
      isPrimary: false,
    });
    await apiClient.seedSessionRoute({
      task_id: task.id,
      destination_workflow_step_id: plan.id,
      destination_session_id: destinationSessionId,
      agent_profile_id: seedData.agentProfileId,
      outcome: "created",
      reason: "no_reusable_candidate",
    });

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // Panels picker → Task History (no bottom-nav item).
    const panelsButton = testPage.getByRole("button", { name: "Panels", exact: true });
    await expect(panelsButton).toBeVisible({ timeout: 15_000 });
    await panelsButton.tap();
    const historyOption = testPage.getByTestId("mobile-task-history-option");
    await expect(historyOption).toBeVisible({ timeout: 10_000 });
    expect((await historyOption.boundingBox())?.height ?? 0).toBeGreaterThanOrEqual(44);
    await historyOption.tap();

    const panel = testPage.getByTestId("mobile-task-history-panel");
    await expect(panel).toBeVisible();
    await expect(panel.getByTestId("task-history-row-transition").first()).toBeVisible();

    // Expanded header step visit → step-filtered timeline; the drawer dismisses
    // so focus lands in the launched surface.
    const expand = testPage.getByTestId("workflow-stepper-expand");
    await expect(expand).toBeVisible({ timeout: 15_000 });
    await expand.tap();
    const drawer = testPage.getByTestId("workflow-transition-drawer");
    await expect(drawer).toBeVisible();
    await drawer.getByTestId(`workflow-step-visit-history-${plan.id}`).tap();
    await expect(drawer).toBeHidden();

    await expect(panel).toBeVisible();
    const transitionRows = panel.getByTestId("task-history-row-transition");
    await expect.poll(async () => transitionRows.count()).toBeGreaterThanOrEqual(2);
    const texts = await transitionRows.allTextContents();
    expect(texts.every((text) => text.includes("Plan"))).toBe(true);

    const routeRow = panel.getByTestId("task-history-row-route");
    await expect(routeRow).toBeVisible();
    await expect(routeRow).toContainText("Created session");
    // Open activates the persisted destination session. The phone has no
    // Dockview tree, so activation is the store's active session — the value
    // the mobile layout renders its session surface from.
    await routeRow.locator('[data-testid^="task-history-open-"]').tap();
    await expect.poll(() => readActiveSessionId(testPage)).toBe(destinationSessionId);
  });
});
