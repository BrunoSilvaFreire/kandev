import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  DEFAULT_DOCUMENT_REVISION_PAGE_SIZE,
  getTaskDocument,
  listTaskDocumentCatalog,
  listTaskDocumentRevisions,
} from "./task-document-api";

const request = vi.fn();
const getWebSocketClient = vi.fn<() => { request: typeof request } | null>(() => ({ request }));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => getWebSocketClient(),
}));

beforeEach(() => {
  request.mockReset();
  getWebSocketClient.mockClear();
});

describe("task document API", () => {
  it("requests the merged catalog for a task", async () => {
    request.mockResolvedValueOnce({ groups: [{ entries: [] }] });
    await expect(listTaskDocumentCatalog("task-1")).resolves.toEqual([{ entries: [] }]);
    expect(request).toHaveBeenCalledWith("task.documents.catalog", { task_id: "task-1" });
  });

  it("requires a WebSocket client", async () => {
    getWebSocketClient.mockReturnValueOnce(null);
    await expect(listTaskDocumentCatalog("task-1")).rejects.toThrow(
      "WebSocket client not available",
    );
  });

  it("fetches a selected document by key", async () => {
    request.mockResolvedValueOnce({ key: "spec", content: "body" });
    await expect(getTaskDocument("task-1", "spec")).resolves.toEqual({
      key: "spec",
      content: "body",
    });
    expect(request).toHaveBeenCalledWith("task.document.get", { task_id: "task-1", key: "spec" });
  });

  it("pages revisions with the default limit and an exclusive cursor", async () => {
    request.mockResolvedValueOnce({ revisions: [{ id: "r1", revision_number: 3 }] });
    const page = await listTaskDocumentRevisions("task-1", "spec", { beforeRevision: 4 });
    expect(page.revisions).toHaveLength(1);
    expect(request).toHaveBeenCalledWith("task.document.revisions.list", {
      task_id: "task-1",
      key: "spec",
      limit: DEFAULT_DOCUMENT_REVISION_PAGE_SIZE,
      before_revision: 4,
    });
  });

  it("omits the cursor when no revision is provided", async () => {
    request.mockResolvedValueOnce({});
    await listTaskDocumentRevisions("task-1", "spec");
    expect(request).toHaveBeenCalledWith("task.document.revisions.list", {
      task_id: "task-1",
      key: "spec",
      limit: DEFAULT_DOCUMENT_REVISION_PAGE_SIZE,
    });
  });
});
