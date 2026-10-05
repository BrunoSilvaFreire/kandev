import { beforeEach, describe, expect, it } from "vitest";
import { useCommentsStore, type DocumentComment } from "@/lib/state/slices/comments";
import { COMMENTS_STORAGE_PREFIX } from "./persistence";

const SESSION_ID = "session-doc";

function documentComment(id = "doc-1"): DocumentComment {
  return {
    id,
    sessionId: SESSION_ID,
    source: "document",
    documentKey: "spec",
    revision: 2,
    selectedText: "retry limit",
    text: "Confirmed?",
    createdAt: "2026-09-25T00:00:00Z",
    status: "pending",
  };
}

function resetStore() {
  resetStoreState();
  window.sessionStorage.clear();
}

function resetStoreState() {
  useCommentsStore.setState({
    byId: {},
    bySession: {},
    pendingForChat: [],
    editingCommentId: null,
  });
}

describe("document comment store round trip", () => {
  beforeEach(resetStore);

  it("adds, persists, rehydrates, and clears on send", () => {
    useCommentsStore.getState().addComment(documentComment());
    expect(useCommentsStore.getState().pendingForChat).toEqual(["doc-1"]);
    const raw = window.sessionStorage.getItem(`${COMMENTS_STORAGE_PREFIX}${SESSION_ID}`);
    expect(raw).toContain("retry limit");

    // A fresh store rehydrates the pending draft from sessionStorage.
    resetStoreState();
    useCommentsStore.getState().hydrateSession(SESSION_ID);
    expect(useCommentsStore.getState().byId["doc-1"]?.source).toBe("document");
    expect(useCommentsStore.getState().pendingForChat).toEqual(["doc-1"]);

    // Sending clears the comment from the store and storage.
    useCommentsStore.getState().markCommentsSent(["doc-1"]);
    expect(useCommentsStore.getState().byId["doc-1"]).toBeUndefined();
    expect(window.sessionStorage.getItem(`${COMMENTS_STORAGE_PREFIX}${SESSION_ID}`)).toBeNull();
  });
});
