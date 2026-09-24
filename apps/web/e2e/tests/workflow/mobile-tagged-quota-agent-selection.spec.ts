import { test, expect } from "../../fixtures/test-base";
import { useRegularMode } from "../../helpers/regular-mode";
import { WorkflowSettingsPage } from "../../pages/workflow-settings-page";

// Configures a step's allowed tags on a phone viewport; run with office off.
useRegularMode();

test.describe("Mobile tagged quota agent selection", () => {
  test("saves and reloads a step's allowed tags", async ({ testPage, seedData, apiClient }) => {
    test.setTimeout(120_000);
    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId);

    const card = await page.findWorkflowCard("E2E Workflow");
    await expect(card).toBeVisible();

    const step = seedData.steps[0];
    expect(step?.id).toBeDefined();
    await page.stepNodeByName(card, step!.name).click();

    const input = testPage.getByTestId(`${step!.id}-allowed-tags-input`);
    await expect(input).toBeVisible({ timeout: 15_000 });
    await input.fill("review");
    await input.press("Enter");
    await expect(testPage.getByTestId(`${step!.id}-allowed-tags-list`)).toContainText("review");

    await page.saveChanges();

    const persisted = (
      await apiClient.listWorkflowSteps(seedData.workflowId)
    ).steps.find((item) => item.id === step!.id);
    expect(persisted?.allowed_tags).toEqual(["review"]);

    await page.goto(seedData.workspaceId);
    const reloadedCard = await page.findWorkflowCard("E2E Workflow");
    await page.stepNodeByName(reloadedCard, step!.name).click();
    await expect(testPage.getByTestId(`${step!.id}-allowed-tags-list`)).toContainText("review", {
      timeout: 15_000,
    });
  });

  test("shows candidate quota states in the picker sheet", async ({
    testPage,
    seedData,
    apiClient,
  }) => {
    test.setTimeout(120_000);
    const { agents } = await apiClient.listAgents();
    const agent = agents[0];
    const high = await apiClient.createAgentProfile(agent.id, "Phone quota high", {
      model: "mock-fast",
      tags: ["phone-review", "mock-quota-90"],
    });
    const low = await apiClient.createAgentProfile(agent.id, "Phone quota low", {
      model: "mock-fast",
      tags: ["phone-review", "mock-quota-10"],
    });
    const step = seedData.steps[0];
    expect(step?.id).toBeDefined();
    let taskId: string | undefined;

    try {
      await apiClient.updateWorkflowStep(step!.id, {
        allowed_tags: ["phone-review"],
        agent_profile_id: high.id,
      });

      const page = new WorkflowSettingsPage(testPage);
      await page.goto(seedData.workspaceId);
      const card = await page.findWorkflowCard("E2E Workflow");
      await page.stepNodeByName(card, step!.name).click();

      const trigger = testPage.getByTestId(`${step!.id}-candidate-preview-trigger`);
      await expect(trigger).toBeVisible({ timeout: 15_000 });
      const overflows = await testPage.evaluate(
        () => document.documentElement.scrollWidth > window.innerWidth + 1,
      );
      expect(overflows).toBe(false);
      const triggerBox = await trigger.boundingBox();
      expect(triggerBox?.height ?? 0).toBeGreaterThanOrEqual(44);

      await trigger.click();
      const content = testPage.getByTestId(`${step!.id}-candidate-preview-sheet-content`);
      await expect(content).toBeVisible();
      await expect(content).toContainText("90% remaining");
      await expect(content).toContainText("10% remaining");
      const closeBox = await testPage
        .getByTestId(`${step!.id}-candidate-preview-close`)
        .boundingBox();
      expect(closeBox?.height ?? 0).toBeGreaterThanOrEqual(44);

      await testPage.getByTestId(`${step!.id}-candidate-preview-close`).click();
      await expect(trigger).toBeFocused({ timeout: 5_000 });

      // Same selection outcome as desktop: a new entry freezes the
      // higher-remaining tagged profile on the phone project too.
      const created = await apiClient.createTask(seedData.workspaceId, "Phone tagged entry", {
        workflow_id: seedData.workflowId,
        workflow_step_id: step!.id,
      });
      taskId = created.id;
      const stored = await apiClient.getTask(created.id);
      const route = stored.metadata?.workflow_session_route as
        | { agent_profile_id?: string }
        | undefined;
      expect(route?.agent_profile_id).toBe(low.id);
    } finally {
      if (taskId) await apiClient.deleteTask(taskId).catch(() => undefined);
      await apiClient.updateWorkflowStep(step!.id, { allowed_tags: [], agent_profile_id: "" });
      await apiClient.deleteAgentProfile(high.id).catch(() => undefined);
      await apiClient.deleteAgentProfile(low.id).catch(() => undefined);
    }
  });
});
