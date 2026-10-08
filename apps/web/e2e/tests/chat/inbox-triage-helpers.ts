import { expect, type Page } from "@playwright/test";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { waitForSessionState } from "../../helpers/session";

export const POSSIBLE_QUESTION_PROSE =
  "I can take approach A or approach B. Which option do you prefer?";

/**
 * Seeds one real clarification and one advisory possible-question task, and
 * waits until both have settled and the possible-question hint is projected.
 */
export async function seedTriageTasks(apiClient: ApiClient, seedData: SeedData) {
  const clarification = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Triage Clarification",
    seedData.agentProfileId,
    {
      description: "/e2e:clarification",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  const question = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Triage Possible Question",
    seedData.agentProfileId,
    {
      description: `e2e:message(${JSON.stringify(POSSIBLE_QUESTION_PROSE)})`,
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  const clarificationSessionId = clarification.session_id;
  const questionSessionId = question.session_id;
  if (!clarificationSessionId || !questionSessionId) {
    throw new Error("expected active sessions for both seeded triage tasks");
  }
  await waitForSessionState(apiClient, {
    taskId: clarification.id,
    sessionId: clarificationSessionId,
    expectedState: "WAITING_FOR_INPUT",
    message: "clarification task should reach its blocking state",
    timeout: 60_000,
  });
  await waitForSessionState(apiClient, {
    taskId: question.id,
    sessionId: questionSessionId,
    expectedState: "WAITING_FOR_INPUT",
    message: "possible-question task should settle into waiting",
    timeout: 60_000,
  });
  // The hint is set by the orchestrator at the settled transition; poll until
  // the structured projection carries it rather than trusting the state alone.
  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(question.id);
        return sessions.find((session) => session.id === questionSessionId)?.possible_question;
      },
      { timeout: 30_000, message: "possible-question hint should project onto the session" },
    )
    .toBe(true);
  return { clarification, question };
}

/** Asserts the ordered lane contents and that the hint opens the conversation. */
export async function assertTriageLane(page: Page, questionTaskId: string) {
  const triageTab = page.getByTestId("inbox-tab-triage");
  await expect(triageTab).toBeVisible();
  await triageTab.click();
  await expect(page).toHaveURL(/tab=triage/);

  const clarificationRow = page.getByTestId("inbox-triage-row-clarification");
  const questionRow = page.getByTestId("inbox-triage-row-possible_question");
  await expect(clarificationRow).toBeVisible({ timeout: 30_000 });
  await expect(questionRow).toBeVisible();

  // A real clarification outranks an advisory hint in the ordered lane.
  const rows = page.locator('[data-testid^="inbox-triage-row-"]');
  await expect(rows.first()).toHaveAttribute("data-testid", "inbox-triage-row-clarification");

  // The hint opens the conversation, not a synthetic answer form.
  await expect(questionRow).toHaveAttribute("href", new RegExp(`/t/${questionTaskId}`));
  return questionRow;
}
