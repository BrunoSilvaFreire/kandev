import { expect, type Page } from "@playwright/test";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";

export const CRITERION_DESCRIPTION = "Ships the reviewed diff";

/** Seeds a task in `stepId` whose completion gate has one unverified criterion. */
export async function seedGateBlockedTask(api: ApiClient, seed: SeedData, stepId: string) {
  const { task_id: taskId } = await api.seedTask(seed.workspaceId, "Gate blocked move", {
    state: "IN_PROGRESS",
    workflow_id: seed.workflowId,
    workflow_step_id: stepId,
  });
  await api.seedTaskSession(taskId, {
    state: "IDLE",
    agentProfileId: seed.agentProfileId,
    repositoryId: seed.repositoryId,
  });
  const criteria = await api.rawRequest("PUT", `/api/v1/tasks/${taskId}/completion-gate/criteria`, {
    expected_revision: 0,
    criteria: [
      {
        id: "C1",
        description: CRITERION_DESCRIPTION,
        evidence_subject: { kind: "plan_increment", id: "C1" },
      },
    ],
  });
  expect(criteria.status, await criteria.text()).toBe(200);
  return taskId;
}

export function doneStep(seed: SeedData) {
  const done = seed.steps.find((step) => step.name === "Done");
  if (!done) throw new Error("seed workflow has no Done step");
  return done;
}

/**
 * Opens a gate-blocked task and tries to move it to the terminal step from
 * the desktop stepper. The backend refuses the move.
 */
export async function attemptGateBlockedMove(page: Page, api: ApiClient, seed: SeedData) {
  const done = doneStep(seed);
  const taskId = await seedGateBlockedTask(api, seed, seed.startStepId);
  await page.goto(`/t/${taskId}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.moveToWorkflowStep(done);
  return { taskId, done };
}
