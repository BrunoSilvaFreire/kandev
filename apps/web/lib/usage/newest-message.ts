import { parseTurnTimestamp } from "@/lib/state/slices/session/turn-actions";
import type { Message } from "@/lib/types/http";

/**
 * Returns the conversation message with the newest `created_at`, breaking ties
 * by id so the result is deterministic. Messages with an unparseable timestamp
 * are ignored.
 */
export function newestMessage(messages: Message[]): Message | null {
  let newest: Message | null = null;
  let newestAt: bigint | null = null;
  for (const message of messages) {
    const at = parseTurnTimestamp(message.created_at);
    if (at === null) continue;
    const isNewer = newestAt === null || at > newestAt;
    const winsTie = at === newestAt && newest !== null && message.id > newest.id;
    if (isNewer || winsTie) {
      newest = message;
      newestAt = at;
    }
  }
  return newest;
}
