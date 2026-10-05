import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { TaskDocumentReviewContent } from "./task-document-review-content";

const mockReview = vi.hoisted(() => ({
  value: {
    detail: null as unknown,
    status: "loading",
    revisions: [] as unknown[],
    revisionsStatus: "success",
    hasMoreRevisions: false,
    loadMoreRevisions: vi.fn(),
    reload: vi.fn(),
  },
}));

vi.mock("@/hooks/domains/task/use-task-documents", () => ({
  useTaskDocumentReview: () => mockReview.value,
}));

vi.mock("@/hooks/domains/task/use-document-source-label", () => ({
  useDocumentSourceLabel: () => null,
}));

const mockStore = vi.hoisted(() => ({ activeSessionId: null as string | null }));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: { tasks: { activeSessionId: string | null } }) => unknown) =>
    selector({ tasks: { activeSessionId: mockStore.activeSessionId } }),
}));

vi.mock("./markdown-preview-content", () => ({
  MarkdownPreviewRenderer: ({ content }: { content: string }) => (
    <div data-testid="markdown-body">{content}</div>
  ),
}));

afterEach(() => {
  cleanup();
  mockStore.activeSessionId = null;
  vi.restoreAllMocks();
});

function mockSelection(container: Element) {
  const range = {
    commonAncestorContainer: container,
    getBoundingClientRect: () => ({ left: 10, top: 20, width: 40, height: 10, bottom: 30 }),
  };
  vi.spyOn(window, "getSelection").mockReturnValue({
    isCollapsed: false,
    toString: () => "Heading",
    getRangeAt: () => range,
    removeAllRanges: vi.fn(),
  } as unknown as Selection);
}

function renderReview() {
  return render(<TaskDocumentReviewContent panelId="plan" taskId="task-1" documentKey="spec" />);
}

const detail = {
  key: "spec",
  title: "Architecture",
  type: "SPEC",
  is_plan: false,
  content: "# Heading",
  latest_revision_number: 4,
  updated_at: "2026-01-01T00:00:00Z",
};

describe("TaskDocumentReviewContent", () => {
  it("renders read-only content with legacy provenance when unknown", () => {
    mockReview.value = {
      detail,
      status: "success",
      revisions: [],
      revisionsStatus: "success",
      hasMoreRevisions: false,
      loadMoreRevisions: vi.fn(),
      reload: vi.fn(),
    };
    render(<TaskDocumentReviewContent panelId="plan" taskId="task-1" documentKey="spec" />);
    expect(screen.getByTestId("markdown-body").textContent).toBe("# Heading");
    expect(screen.getByTestId("document-source-label").textContent).toBe("Unknown source");
    expect(screen.queryByText("Save")).toBeNull();
  });

  it("expands the revision history lazily", () => {
    const loadMore = vi.fn();
    mockReview.value = {
      detail,
      status: "success",
      revisions: [
        {
          id: "r4",
          revision_number: 4,
          title: "Architecture",
          author_name: "agent",
          author_kind: "agent",
          created_at: "2026-01-01T00:00:00Z",
          updated_at: "2026-01-01T00:00:00Z",
        },
      ],
      revisionsStatus: "success",
      hasMoreRevisions: true,
      loadMoreRevisions: loadMore,
      reload: vi.fn(),
    };
    render(<TaskDocumentReviewContent panelId="plan" taskId="task-1" documentKey="spec" />);
    expect(screen.queryByTestId("document-revision-row")).toBeNull();
    fireEvent.click(screen.getByTestId("document-revisions-toggle"));
    expect(screen.getByTestId("document-revision-row")).toBeTruthy();
    fireEvent.click(screen.getByText("Load more revisions"));
    expect(loadMore).toHaveBeenCalled();
  });

  it("renders a retry action on load failure", () => {
    const reload = vi.fn();
    mockReview.value = {
      detail: null,
      status: "error",
      revisions: [],
      revisionsStatus: "idle",
      hasMoreRevisions: false,
      loadMoreRevisions: vi.fn(),
      reload,
    };
    render(<TaskDocumentReviewContent panelId="plan" taskId="task-1" documentKey="spec" />);
    fireEvent.click(screen.getByText("Retry"));
    expect(reload).toHaveBeenCalled();
  });

  it("disables the selection affordance without an active session", async () => {
    mockStore.activeSessionId = null;
    mockReview.value = {
      detail,
      status: "success",
      revisions: [],
      revisionsStatus: "success",
      hasMoreRevisions: false,
      loadMoreRevisions: vi.fn(),
      reload: vi.fn(),
    };
    renderReview();
    const body = screen.getByTestId("task-document-review-body");
    mockSelection(body);
    fireEvent.mouseUp(body);
    expect(await screen.findByTestId("document-comment-disabled")).toHaveProperty("disabled", true);
    expect(screen.queryByTestId("document-selection-popover")).toBeNull();
  });

  it("opens the selection composer with an active session", async () => {
    mockStore.activeSessionId = "session-1";
    mockReview.value = {
      detail,
      status: "success",
      revisions: [],
      revisionsStatus: "success",
      hasMoreRevisions: false,
      loadMoreRevisions: vi.fn(),
      reload: vi.fn(),
    };
    renderReview();
    const body = screen.getByTestId("task-document-review-body");
    mockSelection(body);
    fireEvent.mouseUp(body);
    expect(await screen.findByTestId("document-selection-popover")).toBeTruthy();
    expect(screen.queryByTestId("document-comment-disabled")).toBeNull();
  });
});
