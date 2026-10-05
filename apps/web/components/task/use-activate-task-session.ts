"use client";

import { useCallback } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { addSessionPanel } from "@/lib/state/dockview-panel-actions";
import { markSessionTabUserActivationIntent } from "./session-tab-activation-intent";
import { t } from "@/lib/i18n";

/**
 * The one sanctioned path to activate a task session from a non-session
 * surface (Task History, the header transition summary, a step visit):
 *
 *   1. mark the session-tab user activation intent so the Dockview tab sync
 *      consumes it instead of restoring the previous session;
 *   2. ensure the `session:<id>` panel exists and is active (its activation
 *      fires the sync, which would otherwise be a no-op);
 *   3. set the active session in the app store.
 *
 * On phone there is no Dockview tree, so step 2 is skipped and the store
 * activation alone switches the mobile session surface.
 */
export function useActivateTaskSession(): (sessionId: string, title?: string) => void {
  const appStore = useAppStoreApi();
  const setActiveSession = useAppStore((state) => state.setActiveSession);

  return useCallback(
    (sessionId: string, title?: string) => {
      if (!sessionId) return;
      const state = appStore.getState();
      const taskId = state.tasks.activeTaskId;
      if (!taskId) return;
      const label = title || state.taskSessions.items[sessionId]?.name || t("task:panelAgent");
      markSessionTabUserActivationIntent(sessionId);
      const { api, centerGroupId } = useDockviewStore.getState();
      if (api) addSessionPanel(api, centerGroupId, sessionId, label);
      setActiveSession(taskId, sessionId);
    },
    [appStore, setActiveSession],
  );
}
