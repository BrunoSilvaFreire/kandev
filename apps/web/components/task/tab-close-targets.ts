/**
 * Shared targeting for the tab-strip close actions.
 *
 * The close actions operate on the tabs of one strip in visual order. The
 * structural `chat` panel is never a target: it is a permanent surface, not a
 * conversation tab. Session panels are targets; closing one closes its tab
 * without deleting the session.
 */

export type TabCloseMode = "others" | "right";

/** Ids a close action removes, in strip order. An unknown target closes nothing. */
export function closeTargets(orderedIds: string[], id: string, mode: TabCloseMode): string[] {
  const index = orderedIds.indexOf(id);
  if (index < 0) return [];
  const candidates =
    mode === "others"
      ? orderedIds.filter((panelId) => panelId !== id)
      : orderedIds.slice(index + 1);
  return candidates.filter((panelId) => panelId !== "chat");
}
