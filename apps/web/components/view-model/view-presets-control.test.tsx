import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import type { KanbanViewPresets } from "@/hooks/use-kanban-view-presets";
import type { ViewPreset } from "@/lib/view-model/types";

// The menu is a flat list of buttons here; opening behaviour is Radix's own.
vi.mock("@kandev/ui/dropdown-menu", () => ({
  DropdownMenu: ({ children }: { children: ReactNode }) => <>{children}</>,
  DropdownMenuTrigger: ({ children }: { children: ReactNode }) => <>{children}</>,
  DropdownMenuContent: ({ children }: { children: ReactNode }) => <>{children}</>,
  DropdownMenuItem: ({
    children,
    onSelect,
    disabled,
    className: _className,
    ...props
  }: {
    children: ReactNode;
    onSelect?: () => void;
    disabled?: boolean;
    className?: string;
    [key: string]: unknown;
  }) => (
    <button type="button" {...props} disabled={disabled} onClick={() => onSelect?.()}>
      {children}
    </button>
  ),
}));

import { ViewGroupOrderControl } from "./view-group-order-control";
import { ViewPresetsControl } from "./view-presets-control";

const mine: ViewPreset = { id: "p1", name: "Mine", filters: [], groups: ["state"] };

function presets(overrides: Partial<KanbanViewPresets> = {}): KanbanViewPresets {
  return {
    items: [mine],
    active: mine,
    dirty: false,
    validateName: () => null,
    apply: vi.fn(),
    create: vi.fn(() => null),
    saveActive: vi.fn(),
    rename: vi.fn(() => null),
    remove: vi.fn(),
    ...overrides,
  };
}

describe("ViewPresetsControl", () => {
  afterEach(cleanup);

  it("shows the modified badge only when the live view differs from the preset", () => {
    const { rerender } = render(<ViewPresetsControl presets={presets()} testId="kanban-presets" />);
    expect(screen.queryByTestId("kanban-presets-dirty")).toBeNull();
    rerender(<ViewPresetsControl presets={presets({ dirty: true })} testId="kanban-presets" />);
    expect(screen.getByTestId("kanban-presets-dirty")).toBeTruthy();
    // Still names the active preset while it is re-selectable.
    expect(screen.getByTestId("kanban-presets-select").textContent).toContain("Mine");
  });

  it("keeps the dialog open and explains a rejected name, then saves a valid one", () => {
    const create = vi
      .fn<KanbanViewPresets["create"]>()
      .mockReturnValueOnce("duplicate")
      .mockReturnValueOnce(null);
    render(<ViewPresetsControl presets={presets({ create })} testId="kanban-presets" />);
    fireEvent.click(screen.getByTestId("kanban-presets-save-as"));

    const input = screen.getByTestId("view-preset-name-input");
    fireEvent.change(input, { target: { value: "mine" } });
    fireEvent.click(screen.getByTestId("view-preset-name-save"));
    expect(screen.getByTestId("view-preset-name-error").textContent).toContain("already has");

    fireEvent.change(input, { target: { value: "Reviews" } });
    expect(screen.queryByTestId("view-preset-name-error")).toBeNull();
    fireEvent.click(screen.getByTestId("view-preset-name-save"));
    expect(create).toHaveBeenLastCalledWith("Reviews");
    expect(screen.queryByTestId("view-preset-name-dialog")).toBeNull();
  });

  it("only updates on an explicit action and only when there is something to save", () => {
    const saveActive = vi.fn();
    const { rerender } = render(
      <ViewPresetsControl presets={presets({ saveActive })} testId="kanban-presets" />,
    );
    expect((screen.getByTestId("kanban-presets-update") as HTMLButtonElement).disabled).toBe(true);

    rerender(
      <ViewPresetsControl presets={presets({ saveActive, dirty: true })} testId="kanban-presets" />,
    );
    fireEvent.click(screen.getByTestId("kanban-presets-update"));
    expect(saveActive).toHaveBeenCalledTimes(1);
  });

  it("deletes the active preset from the actions menu", () => {
    const remove = vi.fn();
    render(<ViewPresetsControl presets={presets({ remove })} testId="kanban-presets" />);
    fireEvent.click(screen.getByTestId("kanban-presets-delete"));
    expect(remove).toHaveBeenCalledWith("p1");
  });
});

describe("ViewGroupOrderControl", () => {
  afterEach(cleanup);
  const groups = ["none", "repositoryGroup", "workflow", "state"] as const;

  it("offers a second level only once a first key is chosen", () => {
    const { rerender } = render(
      <ViewGroupOrderControl
        value={["none"]}
        groups={groups}
        onChange={vi.fn()}
        testId="kanban-group"
      />,
    );
    expect(screen.queryByTestId("kanban-group-then-select")).toBeNull();
    rerender(
      <ViewGroupOrderControl
        value={["repositoryGroup"]}
        groups={groups}
        onChange={vi.fn()}
        testId="kanban-group"
      />,
    );
    expect(screen.getByTestId("kanban-group-then-select")).toBeTruthy();
  });
});
