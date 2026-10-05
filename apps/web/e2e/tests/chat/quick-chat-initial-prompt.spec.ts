import { test, expect } from "../../fixtures/test-base";
import {
  openQuickChatSetup,
  selectAgentIfNeeded,
  startQuickChatFromSetup,
} from "./quick-chat-helpers";

/**
 * Quick Chat initial-prompt flow: a chat started with a prompt submits it once
 * the session is ready, and an empty start stays idle.
 */

const INITIAL_PROMPT_PLACEHOLDER = "Describe what you want the agent to do";
const PROMPT = "/e2e:simple-message";

test.describe("Quick Chat initial prompt", () => {
  // @covers AC-UI-QUICK-CHATS-SURFACE-004.2 AC-UI-QUICK-CHATS-SURFACE-004.3
  test("submits the opening prompt without further input", async ({ testPage }) => {
    const dialog = await openQuickChatSetup(testPage);
    await selectAgentIfNeeded(dialog, testPage);

    await dialog.getByPlaceholder(INITIAL_PROMPT_PLACEHOLDER).fill(PROMPT);
    await dialog.getByTestId("quick-chat-start").click();

    // The prompt becomes the first user message…
    await expect(dialog.getByText(PROMPT, { exact: true })).toBeVisible({ timeout: 15_000 });
    // …and the agent replies without any further interaction.
    await expect(dialog.getByText("simple mock response", { exact: false })).toBeVisible({
      timeout: 30_000,
    });
  });

  // @covers AC-UI-QUICK-CHATS-SURFACE-004.4
  test("starts an empty chat when no prompt is provided", async ({ testPage }) => {
    const dialog = await openQuickChatSetup(testPage);
    await startQuickChatFromSetup(dialog, testPage);

    await expect(dialog.getByText(PROMPT, { exact: true })).toHaveCount(0);
    await expect(dialog.getByText("simple mock response", { exact: false })).toHaveCount(0);
  });
});
