import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

const WORKFLOW_NAME = "Task history workflow";
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
        allow_manual_move: true
      - name: Review
        position: 2
        color: bg-yellow-500
        allow_manual_move: true`;

const DESTINATION_SESSION_ID = "sess-history-dest";

test.describe("Task History panel", () => {
  test("shows step visits, filters by step, and retargets Review from a document row", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const imported = await apiClient.importWorkflows(seedData.workspaceId, WORKFLOW_YAML);
    expect(imported.created).toContain(WORKFLOW_NAME);
    const { workflows } = await apiClient.listWorkflows(seedData.workspaceId);
    const workflow = workflows.find(({ name }) => name === WORKFLOW_NAME);
    if (!workflow) throw new Error("task history workflow was not imported");
    const { steps } = await apiClient.listWorkflowSteps(workflow.id);
    const plan = steps.find(({ name }) => name === "Plan");
    const implement = steps.find(({ name }) => name === "Implement");
    if (!plan || !implement) throw new Error("task history workflow is missing steps");

    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: workflow.id,
    });
    const task = await apiClient.seedTask(seedData.workspaceId, "Task history task", {
      workflow_id: workflow.id,
      workflow_step_id: plan.id,
      state: "IN_PROGRESS",
    });
    // A Plan→Implement→Plan→Implement→Plan loop gives Plan two committed visits.
    for (const stepId of [implement.id, plan.id, implement.id, plan.id]) {
      await apiClient.moveTask(task.task_id, workflow.id, stepId);
    }
    await apiClient.writeTaskDocument(task.task_id, "architecture", {
      type: "SPEC",
      title: "architecture.md",
      content: "# Architecture",
    });
    // A primary initiator session plus a non-primary routed destination, and a
    // durable route into the Plan step whose Open must target the destination.
    const initiator = await apiClient.seedTaskSession(task.task_id, {
      state: "IDLE",
      agentProfileId: seedData.agentProfileId,
      isPrimary: true,
    });
    await apiClient.seedTaskSession(task.task_id, {
      state: "IDLE",
      sessionId: DESTINATION_SESSION_ID,
      agentProfileId: seedData.agentProfileId,
      isPrimary: false,
    });
    await apiClient.seedSessionRoute({
      task_id: task.task_id,
      destination_workflow_step_id: plan.id,
      destination_session_id: DESTINATION_SESSION_ID,
      agent_profile_id: seedData.agentProfileId,
      outcome: "created",
      reason: "no_reusable_candidate",
    });

    await testPage.setViewportSize({ width: 1280, height: 800 });
    await testPage.goto(`/t/${task.task_id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // Open the timeline from the Add Panel menu.
    await session.addPanelButton().click();
    await testPage.getByTestId("add-panel-task-history-item").click();
    const panel = testPage.getByTestId("task-history-panel");
    await expect(panel).toBeVisible();
    await expect(panel.getByTestId("task-history-row-transition").first()).toBeVisible();

    // Review on a document revision row retargets the singleton Review panel.
    await expect(panel.getByTestId("task-history-row-document_revision")).toBeVisible();
    await panel.getByTestId("task-history-review-architecture").click();
    await expect(testPage.getByTestId("task-document-review")).toBeVisible();
    await expect(testPage.getByTestId("task-document-review-body")).toContainText("Architecture");

    // Step filter: the filtered timeline shows the Plan visits only.
    const expand = testPage.getByTestId("workflow-stepper-expand");
    await expand.click();
    const popover = testPage.getByTestId("workflow-transition-popover");
    await expect(popover).toBeVisible();
    await popover.getByTestId(`workflow-step-visit-history-${plan.id}`).click();
    await expect(panel).toBeVisible();

    const transitionRows = panel.getByTestId("task-history-row-transition");
    await expect.poll(async () => transitionRows.count()).toBeGreaterThanOrEqual(2);
    const texts = await transitionRows.allTextContents();
    expect(texts.every((text) => text.includes("Plan"))).toBe(true);

    // Newest-first: transition event ids are monotonic, so they must descend.
    const eventIds = await transitionRows.evaluateAll((rows) =>
      rows
        .map((row) => row.getAttribute("data-event-id") ?? "")
        .filter((id) => id.startsWith("transition:"))
        .map((id) => Number(id.split(":")[1])),
    );
    for (let i = 1; i < eventIds.length; i += 1) {
      expect(eventIds[i - 1]).toBeGreaterThan(eventIds[i]);
    }

    // The step's durable routing decision renders with its profile and reason.
    const routeRow = panel.getByTestId("task-history-row-route");
    await expect(routeRow).toBeVisible();
    await expect(routeRow).toContainText("Created session");
    await expect(routeRow).toContainText("No reusable session candidate");
    // The expandable Details block exposes the raw closed-set decision fields.
    await routeRow.getByTestId("task-history-details").locator("summary").click();
    await expect(routeRow.getByTestId("task-history-details")).toContainText(
      "no_reusable_candidate",
    );
    await expect(panel.getByTestId("task-history-row-document_revision")).toHaveCount(0);

    // Open activates the persisted destination session (not the initiator):
    // the destination's dockview tab becomes active and the initiator's does
    // not.
    await routeRow.locator('[data-testid^="task-history-open-"]').click();
    const destinationTab = testPage.getByTestId(`session-tab-${DESTINATION_SESSION_ID}`);
    await expect(destinationTab).toBeVisible();
    await expect(
      destinationTab.locator('xpath=ancestor::*[contains(@class, "dv-tab")][1]'),
    ).toHaveClass(/dv-active-tab/);
    const initiatorTab = testPage.getByTestId(`session-tab-${initiator.session_id}`);
    await expect(initiatorTab).toBeVisible();
    await expect(
      initiatorTab.locator('xpath=ancestor::*[contains(@class, "dv-tab")][1]'),
    ).not.toHaveClass(/dv-active-tab/);
  });
});
