import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { TaskDocumentCatalogGroup } from "@/lib/types/task-document";
import type { ContextItem } from "@/lib/types/context";
import { EnhanceContextPicker } from "./enhance-context-picker";

const mocks = vi.hoisted(() => ({
  catalog: [] as TaskDocumentCatalogGroup[],
  getTaskDocument: vi.fn(),
  state: {
    taskSessionsByTask: {
      itemsByTaskId: {
        "task-1": [{ id: "s1", name: "Impl session" }],
      },
    },
    messages: {
      bySession: {
        s1: [
          { author_type: "user", content: "make providers fall back differently" },
          { author_type: "agent", content: "implemented the fallback ordering" },
        ],
      },
    },
  },
}));

vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mocks.state) => unknown) => selector(mocks.state),
}));
vi.mock("@/hooks/domains/task/use-task-documents", () => ({
  useTaskDocumentsCatalog: () => ({ groups: mocks.catalog, status: "success", reload: vi.fn() }),
}));
vi.mock("@/lib/api/domains/task-document-api", () => ({
  getTaskDocument: (...args: unknown[]) => mocks.getTaskDocument(...args),
}));

const contextItems: ContextItem[] = [
  {
    kind: "document-comment",
    id: "document-comments",
    label: "Document comments (1)",
    comments: [
      {
        id: "c1",
        sessionId: "s1",
        source: "document",
        documentKey: "spec",
        revision: 2,
        selectedText: "retry limit",
        text: "confirm this",
        createdAt: "2026-09-25T00:00:00Z",
        status: "pending",
      },
    ],
  },
];

afterEach(cleanup);

describe("EnhanceContextPicker", () => {
  it("confirms exactly the selected items, none by default", async () => {
    mocks.catalog = [
      {
        entries: [
          {
            key: "spec",
            title: "Spec doc",
            type: "SPEC",
            is_plan: false,
            latest_revision_number: 2,
            updated_at: "2026-09-25T00:00:00Z",
          },
        ],
      },
    ];
    mocks.getTaskDocument.mockResolvedValue({
      key: "spec",
      title: "Spec doc",
      content: "spec body",
    });

    const onConfirm = vi.fn();
    render(
      <EnhanceContextPicker taskId="task-1" contextItems={contextItems} onConfirm={onConfirm} />,
    );

    // Nothing is selected by default.
    fireEvent.click(screen.getByTestId("enhance-context-confirm"));
    await waitFor(() => expect(onConfirm).toHaveBeenCalledWith([]));
    onConfirm.mockClear();

    fireEvent.click(screen.getByText("Document comments (1)"));
    fireEvent.click(screen.getByText("Spec doc"));
    fireEvent.click(screen.getByText("Impl session"));
    fireEvent.click(screen.getByTestId("enhance-context-confirm"));

    await waitFor(() => expect(onConfirm).toHaveBeenCalledTimes(1));
    const items = onConfirm.mock.calls[0][0] as { kind: string; label: string; text: string }[];
    expect(items).toHaveLength(3);
    expect(items.map((item) => item.kind)).toEqual(["document_selection", "document", "session"]);
    expect(items[0].text).toContain("retry limit");
    expect(items[1]).toMatchObject({ label: "Spec doc", text: "spec body" });
    expect(items[2].text).toContain("make providers fall back differently");
    expect(items[2].text).toContain("implemented the fallback ordering");
  });
});
