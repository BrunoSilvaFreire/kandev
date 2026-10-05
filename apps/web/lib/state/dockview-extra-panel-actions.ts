import { focusOrAddPanel } from "./dockview-layout-builders";
import {
  addSessionPanel,
  addSidePanel,
  buildReviewPanelActions,
  type SidePanelOpts,
  type StoreGet,
  type StoreSet,
} from "./dockview-panel-actions";
import { buildTerminalPanelActions } from "./dockview-terminal-panel-actions";
import {
  DOCUMENTS_PANEL_ID,
  TASK_HISTORY_PANEL_ID,
  USAGE_PANEL_ID,
} from "./layout-manager/constants";
import { panelTitle } from "./layout-manager/panel-title";
import {
  parsePluginPanelId,
  pluginPanelId,
  PLUGIN_PANEL_COMPONENT,
  PLUGIN_PANEL_TAB_COMPONENT,
} from "./layout-manager/plugin-panels";

let scrollTargetToken = 0;

/**
 * Build the transcript actions: `scrollTranscriptToMessage` activates (or
 * adds) the session's chat panel and records a scroll target;
 * `clearScrollTarget`/`clearScrollTargetForOwner` clear it;
 * `addVscodePanel`/`openInternalVscode` open or focus the VSCode panel.
 */
function buildTranscriptActions(set: StoreSet, get: StoreGet) {
  return {
    scrollTranscriptToMessage: (sessionId: string, messageId: string, title: string): boolean => {
      const { api: dockviewApi, centerGroupId } = get();
      if (!dockviewApi) return false;
      const sessionPanelId = `session:${sessionId}`;
      const targetPanel = dockviewApi.getPanel(sessionPanelId) ?? dockviewApi.getPanel("chat");
      if (targetPanel) {
        targetPanel.api.setActive();
      } else {
        addSessionPanel(dockviewApi, centerGroupId, sessionId, title);
      }
      scrollTargetToken += 1;
      set({
        scrollTarget: {
          sessionId,
          messageId,
          token: scrollTargetToken,
          hostPanelId: targetPanel?.id ?? sessionPanelId,
        },
      });
      return true;
    },
    clearScrollTarget: (token: number) => {
      if (get().scrollTarget?.token === token) set({ scrollTarget: null });
    },
    clearScrollTargetForOwner: (sessionId: string, hostPanelId: string) => {
      const target = get().scrollTarget;
      if (target?.sessionId === sessionId && target.hostPanelId === hostPanelId) {
        set({ scrollTarget: null });
      }
    },
    addVscodePanel: () => {
      const { api, centerGroupId } = get();
      if (!api) return;
      focusOrAddPanel(api, {
        id: "vscode",
        component: "vscode",
        title: panelTitle("vscode"),
        position: { referenceGroup: centerGroupId },
      });
    },
    openInternalVscode: (_goto: { file: string; line: number; col: number } | null) => {
      const { api, centerGroupId } = get();
      if (!api) return;
      const existing = api.getPanel("vscode");
      if (existing) {
        existing.api.setActive();
        return;
      }
      focusOrAddPanel(api, {
        id: "vscode",
        component: "vscode",
        title: panelTitle("vscode"),
        position: { referenceGroup: centerGroupId },
      });
    },
  };
}

/**
 * Build the single-instance side-panel actions (plan, plugin task panel,
 * todos) via shared placement rules, plus
 * `closePluginPanels` which removes every open panel contributed by a plugin.
 */
function buildSidePanelActions(get: StoreGet) {
  return {
    addPlanPanel: (opts?: SidePanelOpts) => {
      const { api, centerGroupId } = get();
      if (!api) return;
      addSidePanel(
        api,
        centerGroupId,
        { id: "plan", component: "plan", title: panelTitle("plan"), tabComponent: "planTab" },
        opts,
      );
    },
    addPluginPanel: (pluginId: string, panelKey: string, title: string, opts?: SidePanelOpts) => {
      const { api, centerGroupId } = get();
      if (!api) return;
      addSidePanel(
        api,
        centerGroupId,
        {
          id: pluginPanelId(pluginId, panelKey),
          component: PLUGIN_PANEL_COMPONENT,
          title,
          tabComponent: PLUGIN_PANEL_TAB_COMPONENT,
          params: { pluginId, panelKey },
        },
        opts,
      );
    },
    closePluginPanels: (pluginId: string) => {
      const { api } = get();
      if (!api) return;
      api.panels
        .filter((panel) => parsePluginPanelId(panel.id)?.pluginId === pluginId)
        .forEach((panel) => api.removePanel(panel));
    },
    addTodosPanel: (opts?: SidePanelOpts) => {
      const { api, centerGroupId } = get();
      if (!api) return;
      addSidePanel(
        api,
        centerGroupId,
        { id: "todos", component: "todos", title: panelTitle("todos") },
        opts,
      );
    },
    addBackgroundWorkPanel: (
      opts?: SidePanelOpts & { sessionId?: string; workId?: string; title?: string },
    ) => {
      const { api, centerGroupId } = get();
      if (!api) return;
      const sId = opts?.sessionId || "";
      let id = "background-work";
      if (opts?.workId) {
        id = sId ? `background-work:${sId}:${opts.workId}` : `background-work:${opts.workId}`;
      } else if (sId) {
        id = `background-work:${sId}`;
      }
      const title = opts?.title || panelTitle("background-work");
      addSidePanel(
        api,
        centerGroupId,
        {
          id,
          component: "background-work",
          title,
          params: { sessionId: sId, workId: opts?.workId },
        },
        opts,
      );
    },
    addUsagePanel: (opts?: SidePanelOpts) => {
      const { api, centerGroupId } = get();
      if (!api) return;
      addSidePanel(
        api,
        centerGroupId,
        {
          id: USAGE_PANEL_ID,
          component: USAGE_PANEL_ID,
          title: panelTitle(USAGE_PANEL_ID),
        },
        opts,
      );
    },
    addDocumentsPanel: (opts?: SidePanelOpts) => {
      const { api, centerGroupId } = get();
      if (!api) return;
      addSidePanel(
        api,
        centerGroupId,
        {
          id: DOCUMENTS_PANEL_ID,
          component: DOCUMENTS_PANEL_ID,
          title: panelTitle(DOCUMENTS_PANEL_ID),
        },
        opts,
      );
    },
  };
}

/**
 * Build the Documents/Review actions. The Review surface is the existing
 * `plan` singleton parameterized by `documentKey`: retarget it when open,
 * otherwise open it in review mode.
 */
function buildDocumentsPanelActions(get: StoreGet) {
  return {
    openDocumentReview: (documentKey: string) => {
      const { api, centerGroupId } = get();
      if (!api) return;
      const existing = api.getPanel("plan");
      if (existing) {
        existing.api.updateParameters({ documentKey });
        existing.api.setActive();
        return;
      }
      addSidePanel(api, centerGroupId, {
        id: "plan",
        component: "plan",
        title: panelTitle("plan"),
        tabComponent: "planTab",
        params: { documentKey },
      });
    },
  };
}

/**
 * Build the Task History actions. The singleton panel is retargeted with an
 * optional `stepId` filter when open, otherwise opened beside the chat.
 */
function buildTaskHistoryActions(get: StoreGet) {
  return {
    openTaskHistory: (stepId?: string) => {
      const { api, centerGroupId } = get();
      if (!api) return;
      const params = stepId ? { stepId } : undefined;
      const existing = api.getPanel(TASK_HISTORY_PANEL_ID);
      if (existing) {
        existing.api.updateParameters(params ?? {});
        existing.api.setActive();
        return;
      }
      addSidePanel(api, centerGroupId, {
        id: TASK_HISTORY_PANEL_ID,
        component: TASK_HISTORY_PANEL_ID,
        title: panelTitle(TASK_HISTORY_PANEL_ID),
        params,
      });
    },
  };
}

/**
 * Build the store's extra panel actions: transcript, side-panel, review, and
 * terminal actions. Both store accessors are required so stateful actions
 * cannot silently degrade to no-op setters in test or alternate compositions.
 */
export function buildExtraPanelActions(set: StoreSet, get: StoreGet) {
  return {
    ...buildTranscriptActions(set, get),
    ...buildSidePanelActions(get),
    ...buildDocumentsPanelActions(get),
    ...buildTaskHistoryActions(get),
    ...buildReviewPanelActions(get),
    ...buildTerminalPanelActions(get),
  };
}
