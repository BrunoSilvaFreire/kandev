import { test, expect } from "../../fixtures/test-base";
import { useRegularMode } from "../../helpers/regular-mode";
import { WorkflowSettingsPage } from "../../pages/workflow-settings-page";

// Configures a step's allowed tags through the UI; run with office off.
useRegularMode();

test.describe("Tagged quota agent selection", () => {
  test("saves a step's allowed tags and shows them after reload", async ({
    testPage,
    seedData,
    apiClient,
  }) => {
    test.setTimeout(120_000);
    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId);

    const card = await page.findWorkflowCard("E2E Workflow");
    await expect(card).toBeVisible();

    const step = seedData.steps[0];
    expect(step?.id).toBeDefined();
    const stepNode = page.stepNodeByName(card, step!.name);
    await stepNode.click();

    const input = testPage.getByTestId(`${step!.id}-allowed-tags-input`);
    await expect(input).toBeVisible({ timeout: 15_000 });
    await input.fill("review");
    await input.press("Enter");
    await input.fill("security");
    await input.press("Enter");

    const list = testPage.getByTestId(`${step!.id}-allowed-tags-list`);
    await expect(list).toContainText("review");
    await expect(list).toContainText("security");

    await page.saveChanges();

    const persisted = (
      await apiClient.listWorkflowSteps(seedData.workflowId)
    ).steps.find((item) => item.id === step!.id);
    expect(persisted?.allowed_tags).toEqual(["review", "security"]);

    await page.goto(seedData.workspaceId);
    const reloadedCard = await page.findWorkflowCard("E2E Workflow");
    await page.stepNodeByName(reloadedCard, step!.name).click();
    await expect(testPage.getByTestId(`${step!.id}-allowed-tags-list`)).toContainText("review", {
      timeout: 15_000,
    });
  });

  test("freezes the higher-remaining tagged profile for a new entry", async ({
    seedData,
    apiClient,
    backend,
    testPage,
  }) => {
    test.setTimeout(120_000);
    const { agents } = await apiClient.listAgents();
    const agent = agents[0];
    // mock-quota tags are read only under KANDEV_E2E_MOCK (the e2e profile).
    const highUsed = await apiClient.createAgentProfile(agent.id, "Quota high used", {
      model: "mock-fast",
      tags: ["review", "mock-quota-90"],
    });
    const lowUsed = await apiClient.createAgentProfile(agent.id, "Quota low used", {
      model: "mock-fast",
      tags: ["review", "mock-quota-10"],
    });
    const step = seedData.steps[0];
    expect(step?.id).toBeDefined();
    let taskId: string | undefined;

    try {
      await apiClient.updateWorkflowStep(step!.id, {
        allowed_tags: ["review"],
        agent_profile_id: highUsed.id,
      });

      const response = await testPage.request.post(
        `${backend.baseUrl}/api/v1/agent-profiles/utilization`,
        { data: { profile_ids: [highUsed.id, lowUsed.id] } },
      );
      expect(response.ok()).toBe(true);
      const body = (await response.json()) as {
        profiles: Array<{ profile_id: string; state: string; remaining_pct?: number }>;
      };
      const byId = Object.fromEntries(body.profiles.map((item) => [item.profile_id, item]));
      expect(byId[highUsed.id]?.state).toBe("known");
      expect(byId[highUsed.id]?.remaining_pct).toBe(10);
      expect(byId[lowUsed.id]?.state).toBe("known");
      expect(byId[lowUsed.id]?.remaining_pct).toBe(90);

      const created = await apiClient.createTask(seedData.workspaceId, "Tagged quota entry", {
        workflow_id: seedData.workflowId,
        workflow_step_id: step!.id,
      });
      taskId = created.id;
      const stored = await apiClient.getTask(created.id);
      const route = stored.metadata?.workflow_session_route as
        | { agent_profile_id?: string }
        | undefined;
      expect(route?.agent_profile_id).toBe(lowUsed.id);
    } finally {
      if (taskId) await apiClient.deleteTask(taskId).catch(() => undefined);
      await apiClient.updateWorkflowStep(step!.id, { allowed_tags: [], agent_profile_id: "" });
      await apiClient.deleteAgentProfile(highUsed.id).catch(() => undefined);
      await apiClient.deleteAgentProfile(lowUsed.id).catch(() => undefined);
    }
  });

  test("uses the configured fallback profile when no candidate matches the tags", async ({
    seedData,
    apiClient,
  }) => {
    test.setTimeout(120_000);
    const { agents } = await apiClient.listAgents();
    const agent = agents[0];
    const fallback = await apiClient.createAgentProfile(agent.id, "Quota fallback", {
      model: "mock-fast",
      tags: ["unrelated"],
    });
    const step = seedData.steps[0];
    expect(step?.id).toBeDefined();
    let taskId: string | undefined;

    try {
      await apiClient.updateWorkflowStep(step!.id, {
        allowed_tags: ["missing-tag"],
        agent_profile_id: fallback.id,
      });

      const created = await apiClient.createTask(seedData.workspaceId, "Fallback tagged entry", {
        workflow_id: seedData.workflowId,
        workflow_step_id: step!.id,
      });
      taskId = created.id;
      const stored = await apiClient.getTask(created.id);
      const route = stored.metadata?.workflow_session_route as
        | { agent_profile_id?: string }
        | undefined;
      expect(route?.agent_profile_id).toBe(fallback.id);
    } finally {
      if (taskId) await apiClient.deleteTask(taskId).catch(() => undefined);
      await apiClient.updateWorkflowStep(step!.id, { allowed_tags: [], agent_profile_id: "" });
      await apiClient.deleteAgentProfile(fallback.id).catch(() => undefined);
    }
  });

  test("blocks a new entry when every tagged candidate is exhausted and no fallback is safe", async ({
    seedData,
    apiClient,
    testPage,
  }) => {
    test.setTimeout(120_000);
    const { agents } = await apiClient.listAgents();
    const agent = agents[0];
    const exhausted = await apiClient.createAgentProfile(agent.id, "Quota exhausted", {
      model: "mock-fast",
      tags: ["exhausted", "mock-quota-100"],
    });
    const step = seedData.steps[0];
    expect(step?.id).toBeDefined();

    try {
      await apiClient.updateWorkflowStep(step!.id, {
        allowed_tags: ["exhausted"],
        agent_profile_id: "",
      });

      await expect(
        apiClient.createTask(seedData.workspaceId, "Exhausted tagged entry", {
          workflow_id: seedData.workflowId,
          workflow_step_id: step!.id,
        }),
      ).rejects.toThrow(/eligible/i);

      // Visible recovery: the step editor shows the exhausted candidate's true
      // state so the user knows to change the tags or add a fallback.
      const page = new WorkflowSettingsPage(testPage);
      await page.goto(seedData.workspaceId);
      const card = await page.findWorkflowCard("E2E Workflow");
      await page.stepNodeByName(card, step!.name).click();
      const preview = testPage.getByTestId(`${step!.id}-candidate-preview`);
      await expect(preview).toBeVisible({ timeout: 15_000 });
      await expect(preview).toContainText("0% remaining");
    } finally {
      await apiClient.updateWorkflowStep(step!.id, { allowed_tags: [], agent_profile_id: "" });
      await apiClient.deleteAgentProfile(exhausted.id).catch(() => undefined);
    }
  });
});
