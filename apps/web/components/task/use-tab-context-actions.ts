"use client";

import { useCallback } from "react";
import type { IDockviewPanelHeaderProps } from "dockview-react";
import { removeSessionPanel } from "@/lib/state/dockview-panel-actions";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { markEnvSessionClosed } from "@/lib/dockview-closed-sessions";
import { closeTargets, type TabCloseMode } from "./tab-close-targets";

/**
 * The close actions every dockview tab context menu offers.
 *
 * `Close` matters beyond convenience: a group narrow enough to push a tab's
 * close button toward the edge of the strip leaves the context menu as the
 * reachable way to close it.
 *
 * Session panels close their tab without deleting the session. The structural
 * `chat` panel is spared by Close Others and Close Tabs to the Right.
 */
export function useTabContextActions(
  api: IDockviewPanelHeaderProps["api"],
  containerApi: IDockviewPanelHeaderProps["containerApi"],
) {
  const closePanel = useCallback(
    (panelId: string) => {
      if (panelId.startsWith("session:")) {
        const sessionId = panelId.slice("session:".length);
        markEnvSessionClosed(useDockviewStore.getState().currentLayoutEnvId, sessionId);
        removeSessionPanel(containerApi, sessionId);
        return;
      }
      const panel = containerApi.getPanel(panelId);
      if (panel) containerApi.removePanel(panel);
    },
    [containerApi],
  );

  const closeMany = useCallback(
    (mode: TabCloseMode) => {
      const orderedIds = api.group.panels.map((panel) => panel.id);
      for (const panelId of closeTargets(orderedIds, api.id, mode)) closePanel(panelId);
    },
    [api, closePanel],
  );

  const handleClose = useCallback(() => closePanel(api.id), [api.id, closePanel]);
  const handleCloseOthers = useCallback(() => closeMany("others"), [closeMany]);
  const handleCloseToRight = useCallback(() => closeMany("right"), [closeMany]);

  return { handleClose, handleCloseOthers, handleCloseToRight };
}
