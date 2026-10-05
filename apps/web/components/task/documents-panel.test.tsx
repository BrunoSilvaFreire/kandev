import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { DocumentsPanel } from "./documents-panel";

const mockCatalog = vi.hoisted(() => ({
  value: { groups: [] as unknown[], status: "success", reload: vi.fn() },
}));

vi.mock("@/hooks/domains/task/use-task-documents", () => ({
  useTaskDocumentsCatalog: () => mockCatalog.value,
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({ taskSessions: { items: { s1: { name: "Session Alpha" } } } }),
}));

afterEach(() => cleanup());

function group(overrides: Record<string, unknown> = {}) {
  return {
    session_id: "s1",
    entries: [
      {
        key: "spec",
        title: "spec.md",
        type: "SPEC",
        is_plan: false,
        latest_revision_number: 4,
        updated_at: "2026-01-01T00:00:00Z",
      },
    ],
    ...overrides,
  };
}

describe("DocumentsPanel", () => {
  it("groups entries by their producing session", () => {
    mockCatalog.value = { groups: [group()], status: "success", reload: vi.fn() };
    render(<DocumentsPanel taskId="task-1" onOpenDocument={vi.fn()} />);
    expect(screen.getByText("Session Alpha")).toBeTruthy();
    expect(screen.getByText("rev 4")).toBeTruthy();
  });

  it("falls back to an Unassigned group for legacy entries", () => {
    mockCatalog.value = {
      groups: [group({ session_id: undefined })],
      status: "success",
      reload: vi.fn(),
    };
    render(<DocumentsPanel taskId="task-1" onOpenDocument={vi.fn()} />);
    expect(screen.getByText("Unassigned")).toBeTruthy();
  });

  it("opens the selected document by key", () => {
    const onOpen = vi.fn();
    mockCatalog.value = { groups: [group()], status: "success", reload: vi.fn() };
    render(<DocumentsPanel taskId="task-1" onOpenDocument={onOpen} />);
    fireEvent.click(screen.getByTestId("document-row-spec"));
    expect(onOpen).toHaveBeenCalledWith("spec");
  });

  it("renders an empty state", () => {
    mockCatalog.value = { groups: [], status: "success", reload: vi.fn() };
    render(<DocumentsPanel taskId="task-1" onOpenDocument={vi.fn()} />);
    expect(screen.getByText("No documents yet.")).toBeTruthy();
  });

  it("renders a retry action on failure", () => {
    const reload = vi.fn();
    mockCatalog.value = { groups: [], status: "error", reload };
    render(<DocumentsPanel taskId="task-1" onOpenDocument={vi.fn()} />);
    fireEvent.click(screen.getByText("Retry"));
    expect(reload).toHaveBeenCalled();
  });
});
