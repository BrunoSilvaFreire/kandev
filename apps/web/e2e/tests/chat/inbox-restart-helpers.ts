import { expect, type Page } from "@playwright/test";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";

/**
 * Seeds the two shapes a restart must keep apart: a task whose execution was
 * interrupted (the `interrupted_at` marker is the deterministic stand-in for a
 * real crash) and a task genuinely waiting for review.
 */
export async function seedRestartInboxTasks(apiClient: ApiClient, seedData: SeedData) {
  const options = {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  };
  const interrupted = await apiClient.createTask(seedData.workspaceId, "Restart Interrupted", {
    ...options,
    metadata: { interrupted_at: "2026-08-02T10:00:00Z" },
  });
  await apiClient.updateTaskState(interrupted.id, "REVIEW");
  const review = await apiClient.createTask(seedData.workspaceId, "Restart Genuine Review", {
    ...options,
  });
  await apiClient.updateTaskState(review.id, "REVIEW");
  return { interrupted, review };
}

/**
 * Asserts the lane shows exactly one truthful row per task: the interrupted
 * task offers Resume and is never a review, and the genuine review stays one.
 */
export async function assertRestartLane(page: Page, interruptedId: string, reviewId: string) {
  await page.goto("/needs-you-inbox?tab=triage");
  const interruptedRow = page.getByTestId("inbox-triage-row-interrupted");
  const reviewRow = page.getByTestId("inbox-triage-row-review");
  await expect(interruptedRow).toHaveCount(1);
  await expect(reviewRow).toHaveCount(1);
  await expect(page.getByTestId("inbox-triage-row-stale_review")).toHaveCount(0);

  await expect(interruptedRow).toHaveAttribute("href", new RegExp(`/t/${interruptedId}`));
  await expect(interruptedRow.getByTestId("inbox-triage-action")).toHaveText("Resume");
  await expect(interruptedRow.getByTestId("inbox-triage-reason")).toContainText("interrupted");

  await expect(reviewRow).toHaveAttribute("href", new RegExp(`/t/${reviewId}`));
  await expect(reviewRow.getByTestId("inbox-triage-action")).toHaveText("Review");
  await expect(reviewRow.getByTestId("inbox-triage-reason")).toContainText(
    "waiting for your review",
  );
}
