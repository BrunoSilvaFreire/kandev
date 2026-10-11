import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

const WORKFLOW_NAME = "Transition summary workflow";
const DESTINATION_SESSION_ID = "visit-destination-session";
const WORKFLOW_YAML = `version: 1
type: kandev_workflow
workflows:
  - name: ${WORKFLOW_NAME}
    steps:
      - name: Backlog
        position: 0
        color: bg-neutral-400
        allow_manual_move: true
        events:
          on_turn_complete:
            - type: move_to_next
      - name: Implementation
        position: 1
        color: bg-blue-500
        allow_manual_move: true
        events:
          on_turn_complete:
            - type: move_to_next
      - name: Review
        position: 2
        color: bg-yellow-500
        is_start_step: true
        allow_manual_move: true`;

test.describe("Task topbar workflow transitions", () => {
  test("expands the semantic transition summary without header overflow", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    const imported = await apiClient.importWorkflows(seedData.workspaceId, WORKFLOW_YAML);
    expect(imported.created).toContain(WORKFLOW_NAME);
    const { workflows } = await apiClient.listWorkflows(seedData.workspaceId);
    const workflow = workflows.find(({ name }) => name === WORKFLOW_NAME);
    if (!workflow) throw new Error("transition summary workflow was not imported");
    const { steps } = await apiClient.listWorkflowSteps(workflow.id);
    const review = steps.find(({ name }) => name === "Review");
    if (!review) throw new Error("transition summary workflow is missing Review");
    const backlog = steps.find(({ name }) => name === "Backlog");
    if (!backlog) throw new Error("transition summary workflow is missing Backlog");

    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: workflow.id,
    });
    const task = await apiClient.seedTask(seedData.workspaceId, "Transition summary task", {
      workflow_id: workflow.id,
      workflow_step_id: review.id,
      state: "REVIEW",
    });
    // Leave and re-enter Review so the step has a committed visit to list
    // destination sessions under, independent of the genesis row.
    await apiClient.moveTask(task.task_id, workflow.id, backlog.id);
    await apiClient.moveTask(task.task_id, workflow.id, review.id);
    // An initiator plus a routed destination session, and a durable route into
    // Review whose session row must activate the destination on click.
    await apiClient.seedTaskSession(task.task_id, {
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
      destination_workflow_step_id: review.id,
      destination_session_id: DESTINATION_SESSION_ID,
      agent_profile_id: seedData.agentProfileId,
      outcome: "created",
      reason: "explicit_target",
    });

    await testPage.setViewportSize({ width: 1280, height: 800 });
    await testPage.goto(`/t/${task.task_id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const expand = testPage.getByTestId("workflow-stepper-expand");
    await expect(expand).toBeVisible();
    await expect(expand).toHaveAttribute("aria-expanded", "false");
    await expand.click();
    await expect(expand).toHaveAttribute("aria-expanded", "true");

    const popover = testPage.getByTestId("workflow-transition-popover");
    await expect(popover).toBeVisible();
    await expect(popover.getByTestId("workflow-transition-edge").first()).toBeVisible();
    // Review is current and manual-movable: its manual destinations appear.
    await expect(
      popover.locator('[data-testid="workflow-transition-edge"][data-manual="true"]').first(),
    ).toBeVisible();
    // Each edge carries a committed count (0 when never taken).
    await expect(popover.getByTestId("workflow-transition-count").first()).toBeVisible();

    // The expanded surface must not create document-level horizontal overflow.
    const overflow = await testPage.evaluate(
      () => document.documentElement.scrollWidth - window.innerWidth,
    );
    expect(overflow).toBeLessThanOrEqual(1);

    await expand.click();
    await expect(popover).toBeHidden();

    // Reopen and click a routed destination session row: it activates that
    // session's tab.
    await expand.click();
    // Review was entered at activation (visit 1) and again after Backlog
    // (visit 3); sessions created now belong to the current visit.
    const rowIn = (ordinal: number) =>
      popover.getByTestId(
        `workflow-step-visit-session-${review.id}-${ordinal}-${DESTINATION_SESSION_ID}`,
      );
    const sessionRow = rowIn(3);
    await expect(sessionRow).toBeVisible();
    await sessionRow.click();
    const destinationTab = testPage.getByTestId(`session-tab-${DESTINATION_SESSION_ID}`);
    await expect(destinationTab).toBeVisible();
    await expect(
      destinationTab.locator('xpath=ancestor::*[contains(@class, "dv-tab")][1]'),
    ).toHaveClass(/dv-active-tab/);

    // Reassign the session to the earlier Review visit from its tab menu; the
    // association is durable across a reload and a backend restart.
    await destinationTab.click({ button: "right" });
    await testPage.getByRole("menuitem", { name: "Associate with workflow step" }).click();
    await testPage.getByRole("menuitem", { name: /Review\s*Visit 1/ }).click();
    const reopen = async () => {
      await testPage.reload();
      await session.waitForLoad();
      await expand.click();
    };
    await reopen();
    await expect(rowIn(1)).toBeVisible();
    await expect(rowIn(3)).toHaveCount(0);
    await backend.restart();
    await reopen();
    await expect(rowIn(1)).toBeVisible();
    await expect(rowIn(3)).toHaveCount(0);
  });
});
