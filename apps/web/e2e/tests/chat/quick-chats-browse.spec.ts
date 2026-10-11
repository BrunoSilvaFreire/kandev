// Quick Chats browse page: rows are ordered and dated by conversation
// activity, an unnamed chat is recognizable by its first question, and badges
// show only links the chat can prove. The mobile-chrome sibling lives in
// mobile-quick-chats-browse.spec.ts.
import { test, expect } from "../../fixtures/test-base";
import { CREATED_TASK_TITLE, FIRST_QUESTION, seedBrowseChats } from "./quick-chats-browse-helpers";

test.describe("Quick Chats browse", () => {
  test("orders by activity, titles unnamed chats, and shows proven links", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const { named, unnamed, empty, createdTaskId } = await seedBrowseChats(apiClient, seedData);
    const rowOrder = async () =>
      testPage
        .getByTestId("quick-chats-list")
        .locator('[data-testid^="quick-chat-row-"]:not([data-testid="quick-chat-row-title"])')
        .evaluateAll((rows) => rows.map((row) => row.getAttribute("data-testid")));
    const expected = (ids: string[]) => ids.map((id) => `quick-chat-row-${id}`);

    await testPage.goto("/quick-chats");
    const namedRow = testPage.getByTestId(`quick-chat-row-${named.session_id}`);
    await expect(namedRow).toBeVisible({ timeout: 15_000 });

    // Recent first by default: the chat created just now, then the October
    // question, then the September conversation.
    await expect
      .poll(rowOrder)
      .toEqual(expected([empty.session_id, unnamed.session_id, named.session_id]));

    // Dates come from the conversation, shown in absolute form too.
    await expect(testPage.getByTestId(`quick-chat-activity-${named.session_id}`)).toContainText(
      "Sep 2, 2026",
    );
    await expect(testPage.getByTestId(`quick-chat-activity-${unnamed.session_id}`)).toContainText(
      "Oct 5, 2026",
    );
    await expect(testPage.getByTestId(`quick-chat-activity-${empty.session_id}`)).toContainText(
      "No messages yet",
    );

    // A stored name wins; an unnamed chat shows its first question.
    await expect(namedRow).toContainText("Named chat");
    const unnamedRow = testPage.getByTestId(`quick-chat-row-${unnamed.session_id}`);
    await expect(unnamedRow.getByTestId("quick-chat-row-title")).toHaveText(FIRST_QUESTION);

    // Only proven links: the attached repository on one row, the created task
    // on another, nothing on the chat that has neither.
    const rows = testPage.getByTestId("quick-chat-browse-row");
    await expect(
      rows
        .filter({ has: namedRow })
        .getByTestId(`quick-chat-repository-badge-${seedData.repositoryId}`),
    ).toBeVisible();
    const taskBadge = testPage.getByTestId(`quick-chat-task-badge-${createdTaskId}`);
    await expect(taskBadge).toContainText(CREATED_TASK_TITLE);
    await expect(
      rows
        .filter({ has: testPage.getByTestId(`quick-chat-row-${empty.session_id}`) })
        .getByTestId("quick-chat-badges"),
    ).toHaveCount(0);
    await expect(
      rows.filter({ has: unnamedRow }).locator('[data-testid^="quick-chat-repository-badge-"]'),
    ).toHaveCount(0);

    // Oldest first reverses the dated rows.
    await testPage.getByTestId("quick-chats-order").click();
    await testPage.getByRole("option", { name: "Oldest activity first" }).click();
    await expect
      .poll(rowOrder)
      .toEqual(expected([named.session_id, unnamed.session_id, empty.session_id]));

    // A restart rewrites session rows; the conversation dates do not move.
    await backend.restart();
    await testPage.reload();
    await expect(namedRow).toBeVisible({ timeout: 15_000 });
    await expect
      .poll(rowOrder)
      .toEqual(expected([empty.session_id, unnamed.session_id, named.session_id]));
    await expect(testPage.getByTestId(`quick-chat-activity-${named.session_id}`)).toContainText(
      "Sep 2, 2026",
    );

    // The task badge opens the task.
    await taskBadge.click();
    await expect(testPage).toHaveURL(new RegExp(`/t/${createdTaskId}`));
  });
});
