import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => {
  const request = vi.fn();
  return { request, client: null as { request: typeof request } | null };
});

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => mocks.client,
}));

import { requestContextReset, WebSocketUnavailableError } from "./session-reset-api";

describe("requestContextReset", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.client = null;
  });

  it("sends session.reset_context with the session id and a 30s timeout", async () => {
    mocks.request.mockResolvedValue(undefined);
    mocks.client = { request: mocks.request };

    await requestContextReset("session-1");

    expect(mocks.request).toHaveBeenCalledWith(
      "session.reset_context",
      { session_id: "session-1" },
      30000,
    );
  });

  it("rejects with WebSocketUnavailableError when no client is connected", async () => {
    await expect(requestContextReset("session-1")).rejects.toBeInstanceOf(
      WebSocketUnavailableError,
    );
  });
});
