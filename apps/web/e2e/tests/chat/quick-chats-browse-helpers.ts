import { expect } from "@playwright/test";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";

export const FIRST_QUESTION = "How do I rotate the signing key?";
export const CREATED_TASK_TITLE = "Rotate the signing key";

/**
 * Seeds three Quick Chats whose conversation times differ from when their
 * session rows were written: a named chat with an attached repository and old
 * messages, an unnamed chat with a recent question and a task it created, and
 * a named chat with no messages.
 */
export async function seedBrowseChats(api: ApiClient, seed: SeedData) {
  const namedResponse = await api.rawRequest(
    "POST",
    `/api/v1/workspaces/${seed.workspaceId}/quick-chat`,
    {
      title: "Named chat",
      agent_profile_id: seed.agentProfileId,
      repository_id: seed.repositoryId,
    },
  );
  expect(namedResponse.status, await namedResponse.clone().text()).toBeLessThan(300);
  const named = (await namedResponse.json()) as { task_id: string; session_id: string };
  const unnamed = await api.startQuickChat(seed.workspaceId, seed.agentProfileId, "");
  const empty = await api.startQuickChat(seed.workspaceId, seed.agentProfileId, "Empty chat");

  await api.seedSessionMessage(named.session_id, {
    type: "message",
    authorType: "user",
    content: "draft the release notes",
    createdAt: "2026-09-01T12:00:00Z",
    newTurn: true,
  });
  await api.seedSessionMessage(named.session_id, {
    type: "message",
    authorType: "agent",
    content: "Done.",
    createdAt: "2026-09-02T12:00:00Z",
  });
  await api.seedSessionMessage(unnamed.session_id, {
    type: "message",
    authorType: "user",
    content: FIRST_QUESTION,
    createdAt: "2026-10-05T12:00:00Z",
    newTurn: true,
  });
  const created = await api.seedTask(seed.workspaceId, CREATED_TASK_TITLE, {
    parent_id: unnamed.task_id,
    workflow_id: seed.workflowId,
    workflow_step_id: seed.startStepId,
  });
  return { named, unnamed, empty, createdTaskId: created.task_id };
}
