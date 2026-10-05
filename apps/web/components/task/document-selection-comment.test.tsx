import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { useRef } from "react";
import { useCommentsStore } from "@/lib/state/slices/comments";
import { DocumentSelectionComment } from "./document-selection-comment";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

const SESSION_ID = "session-doc";

function Harness({ sessionId }: { sessionId: string | null }) {
  const ref = useRef<HTMLDivElement>(null);
  return (
    <div>
      <div ref={ref} data-testid="doc-body">
        the retry limit is five
      </div>
      <DocumentSelectionComment
        containerRef={ref}
        documentKey="spec"
        revision={4}
        sessionId={sessionId}
      />
    </div>
  );
}

function mockSelection(container: Element) {
  const range = {
    commonAncestorContainer: container,
    getBoundingClientRect: () => ({ left: 10, top: 20, width: 40, height: 10, bottom: 30 }),
  };
  const selection = {
    isCollapsed: false,
    toString: () => "the retry limit is five",
    getRangeAt: () => range,
    removeAllRanges: vi.fn(),
  };
  vi.spyOn(window, "getSelection").mockReturnValue(selection as unknown as Selection);
}

describe("DocumentSelectionComment", () => {
  beforeEach(() => {
    useCommentsStore.setState({
      byId: {},
      bySession: {},
      pendingForChat: [],
      editingCommentId: null,
    });
    window.sessionStorage.clear();
  });
  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it("adds a document comment keyed to the active session", async () => {
    render(<Harness sessionId={SESSION_ID} />);
    const body = screen.getByTestId("doc-body");
    mockSelection(body);
    fireEvent.mouseUp(body);

    const input = await screen.findByTestId("document-selection-input");
    fireEvent.change(input, { target: { value: "please confirm" } });
    fireEvent.click(screen.getByTestId("document-selection-add"));

    await waitFor(() => expect(useCommentsStore.getState().pendingForChat).toHaveLength(1));
    const comment = useCommentsStore.getState().byId[useCommentsStore.getState().pendingForChat[0]];
    expect(comment).toMatchObject({
      source: "document",
      sessionId: SESSION_ID,
      documentKey: "spec",
      revision: 4,
      selectedText: "the retry limit is five",
      text: "please confirm",
    });
  });

  it("disables the affordance without an active session", async () => {
    render(<Harness sessionId={null} />);
    const body = screen.getByTestId("doc-body");
    mockSelection(body);
    fireEvent.mouseUp(body);

    const disabled = await screen.findByTestId("document-comment-disabled");
    expect(disabled).toHaveProperty("disabled", true);
    expect(useCommentsStore.getState().pendingForChat).toHaveLength(0);
  });
});
