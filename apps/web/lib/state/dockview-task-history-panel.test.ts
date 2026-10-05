import { describe, expect, it } from "vitest";
import { buildExtraPanelActions } from "./dockview-extra-panel-actions";
import { makeApi, makeStore, type MockPanel } from "./dockview-panel-actions.test-utils";
import { TASK_HISTORY_PANEL_ID } from "./layout-manager";

describe("openTaskHistory", () => {
  it("adds the singleton panel in the center group", () => {
    const api = makeApi();
    const store = makeStore(api);
    const actions = buildExtraPanelActions(store.set, store.get);

    actions.openTaskHistory();

    expect(api.getPanel(TASK_HISTORY_PANEL_ID)).toMatchObject({
      id: TASK_HISTORY_PANEL_ID,
      api: { component: TASK_HISTORY_PANEL_ID },
    });
  });

  it("carries the destination-step filter on first open", () => {
    const api = makeApi();
    const store = makeStore(api);
    const actions = buildExtraPanelActions(store.set, store.get);

    actions.openTaskHistory("step-plan");

    const panel = api.getPanel(TASK_HISTORY_PANEL_ID) as unknown as MockPanel | undefined;
    expect(panel?.params.stepId).toBe("step-plan");
  });

  it("retargets an already-open panel instead of creating another", () => {
    const api = makeApi();
    const store = makeStore(api);
    const actions = buildExtraPanelActions(store.set, store.get);

    actions.openTaskHistory("step-plan");
    actions.openTaskHistory("step-impl");

    expect(api.panels.filter((panel) => panel.id === TASK_HISTORY_PANEL_ID)).toHaveLength(1);
    const panel = api.getPanel(TASK_HISTORY_PANEL_ID) as unknown as MockPanel | undefined;
    expect(panel?.params.stepId).toBe("step-impl");
    expect(panel?.isActive).toBe(true);
  });
});
