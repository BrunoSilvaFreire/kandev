import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/react";

vi.mock("dockview-react", () => ({
  DockviewDefaultTab: () => null,
}));

vi.mock("./use-tab-maximize", () => ({
  useTabMaximizeOnDoubleClick: () => () => {},
}));

// Render the menu inline so its items are clickable without driving Radix's
// pointer choreography.
vi.mock("@kandev/ui/context-menu", () => ({
  ContextMenu: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  ContextMenuTrigger: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  ContextMenuContent: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  ContextMenuItem: ({
    children,
    onSelect,
    disabled,
  }: {
    children: React.ReactNode;
    onSelect: () => void;
    disabled?: boolean;
  }) => (
    <button type="button" disabled={disabled} onClick={onSelect}>
      {children}
    </button>
  ),
}));

import { ContextMenuTab } from "./tab-context-menu";

const BROWSER = "browser";
const SESSION = "session:abc";
const TERMINAL = "terminal";
const FILES = "files";
const CHAT = "chat";
const BUTTON = "button";
const CLOSE = "Close";
const CLOSE_OTHERS = "Close Others";
const CLOSE_TO_RIGHT = "Close Tabs to the Right";

type Panel = { id: string };

function makeProps(panelId: string, groupPanels: string[], params?: Record<string, unknown>) {
  const removePanel = vi.fn();
  const panels: Panel[] = groupPanels.map((id) => ({ id }));
  const api = { id: panelId, group: { panels } };
  const containerApi = {
    getPanel: (id: string) => panels.find((panel) => panel.id === id),
    removePanel,
  };
  return {
    removePanel,
    props: { api, containerApi, params } as unknown as React.ComponentProps<typeof ContextMenuTab>,
  };
}

describe("ContextMenuTab", () => {
  afterEach(() => cleanup());

  it("closes its own panel from the menu", () => {
    const { props, removePanel } = makeProps(BROWSER, [CHAT, BROWSER, TERMINAL]);
    render(<ContextMenuTab {...props} />);

    fireEvent.click(screen.getByRole(BUTTON, { name: CLOSE }));

    expect(removePanel).toHaveBeenCalledTimes(1);
    expect(removePanel).toHaveBeenCalledWith({ id: BROWSER });
  });

  it("closes siblings but spares only the chat panel", () => {
    const { props, removePanel } = makeProps(BROWSER, [CHAT, SESSION, BROWSER, TERMINAL, FILES]);
    render(<ContextMenuTab {...props} />);

    fireEvent.click(screen.getByRole(BUTTON, { name: CLOSE_OTHERS }));

    expect(removePanel.mock.calls.map(([panel]) => (panel as Panel).id)).toEqual([
      SESSION,
      TERMINAL,
      FILES,
    ]);
  });

  it("closes only the tabs to the right of the target", () => {
    const { props, removePanel } = makeProps(SESSION, [CHAT, SESSION, TERMINAL, FILES]);
    render(<ContextMenuTab {...props} />);

    fireEvent.click(screen.getByRole(BUTTON, { name: CLOSE_TO_RIGHT }));

    expect(removePanel.mock.calls.map(([panel]) => (panel as Panel).id)).toEqual([TERMINAL, FILES]);
  });

  it("renders panel-injected items alongside the close actions", () => {
    const onSelect = vi.fn();
    const { props } = makeProps(BROWSER, [BROWSER], {
      contextMenuItems: [{ label: "Reload", onSelect }],
    });
    render(<ContextMenuTab {...props} />);

    fireEvent.click(screen.getByRole(BUTTON, { name: "Reload" }));

    expect(onSelect).toHaveBeenCalledTimes(1);
    expect(screen.getByRole(BUTTON, { name: CLOSE })).toBeTruthy();
    expect(screen.getByRole(BUTTON, { name: CLOSE_OTHERS })).toBeTruthy();
  });
});
