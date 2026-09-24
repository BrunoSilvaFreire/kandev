import { parseTurnTimestamp } from "@/lib/state/slices/session/turn-actions";
import type { Message } from "@/lib/types/http";

const NANOSECONDS_PER_MILLISECOND = BigInt(1_000_000);

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

/**
 * Epoch milliseconds of the newest message, or null when there is none.
 * `parseTurnTimestamp` returns epoch nanoseconds, so the value is scaled to the
 * millisecond unit the cache heuristic compares against.
 */
export function newestMessageAtMs(messages: Message[]): number | null {
  const newest = newestMessage(messages);
  if (!newest) return null;
  const at = parseTurnTimestamp(newest.created_at);
  return at === null ? null : Number(at / NANOSECONDS_PER_MILLISECOND);
}
