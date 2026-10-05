import { describe, expect, it } from "vitest";
import { buildExtraPanelActions } from "./dockview-extra-panel-actions";
import { makeApi, makeStore, type MockPanel } from "./dockview-panel-actions.test-utils";
import { DOCUMENTS_PANEL_ID } from "./layout-manager";

describe("addDocumentsPanel", () => {
  it("adds the registered catalog panel in the center group", () => {
    const api = makeApi();
    const store = makeStore(api);
    const actions = buildExtraPanelActions(store.set, store.get);

    actions.addDocumentsPanel({ inCenter: true });

    expect(api.getPanel(DOCUMENTS_PANEL_ID)).toMatchObject({
      id: DOCUMENTS_PANEL_ID,
      api: { component: DOCUMENTS_PANEL_ID },
    });
  });
});

describe("openDocumentReview", () => {
  it("opens the plan singleton with the document key when it is not open", () => {
    const api = makeApi();
    const store = makeStore(api);
    const actions = buildExtraPanelActions(store.set, store.get);

    actions.openDocumentReview("architecture.md");

    const panel = api.getPanel("plan") as unknown as MockPanel | undefined;
    expect(panel?.params.documentKey).toBe("architecture.md");
    expect(panel?.api.component).toBe("plan");
  });

  it("retargets an already-open plan singleton instead of creating another", () => {
    const api = makeApi();
    const store = makeStore(api);
    const actions = buildExtraPanelActions(store.set, store.get);

    actions.addPlanPanel();
    actions.openDocumentReview("spike.md");

    expect(api.panels.filter((panel) => panel.id === "plan")).toHaveLength(1);
    const panel = api.getPanel("plan") as unknown as MockPanel | undefined;
    expect(panel?.params.documentKey).toBe("spike.md");
    expect(panel?.isActive).toBe(true);
  });
});
