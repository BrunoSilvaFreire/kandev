import { getWebSocketClient } from "@/lib/ws/connection";

/**
 * Thrown when a context-reset request cannot be sent because the WebSocket
 * client is unavailable. Callers translate it to user-facing copy instead of
 * surfacing the internal message.
 */
export class WebSocketUnavailableError extends Error {
  constructor() {
    // i18n-exempt: internal sentinel; callers map it to translated copy.
    super("WebSocket client unavailable");
    this.name = "WebSocketUnavailableError";
  }
}

/**
 * Sends `session.reset_context` and resolves once the backend has restored the
 * session to `WAITING_FOR_INPUT` with a fresh ACP context. Rejects when the
 * request fails or the connection is unavailable.
 */
export async function requestContextReset(sessionId: string): Promise<void> {
  const client = getWebSocketClient();
  if (!client) {
    throw new WebSocketUnavailableError();
  }
  await client.request("session.reset_context", { session_id: sessionId }, 30000);
}
