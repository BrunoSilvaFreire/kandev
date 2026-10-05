import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import {
  getTaskDocument,
  listTaskDocumentCatalog,
  listTaskDocumentRevisions,
} from "@/lib/api/domains/task-document-api";
import { useTaskDocumentReview, useTaskDocumentsCatalog } from "./use-task-documents";

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({ connection: { status: "connected" } }),
}));

vi.mock("@/lib/api/domains/task-document-api", () => ({
  DEFAULT_DOCUMENT_REVISION_PAGE_SIZE: 20,
  getTaskDocument: vi.fn(),
  listTaskDocumentCatalog: vi.fn(),
  listTaskDocumentRevisions: vi.fn(),
}));

const catalog = vi.mocked(listTaskDocumentCatalog);
const detail = vi.mocked(getTaskDocument);
const revisions = vi.mocked(listTaskDocumentRevisions);

beforeEach(() => {
  catalog.mockReset();
  detail.mockReset();
  revisions.mockReset();
});

afterEach(() => {
  vi.clearAllMocks();
});

describe("useTaskDocumentsCatalog", () => {
  it("stays idle without a task", () => {
    const { result } = renderHook(() => useTaskDocumentsCatalog(null));
    expect(result.current.status).toBe("idle");
    expect(catalog).not.toHaveBeenCalled();
  });

  it("loads the grouped catalog and exposes a reload", async () => {
    catalog.mockResolvedValueOnce([{ session_id: "s1", entries: [] }]);
    const { result } = renderHook(() => useTaskDocumentsCatalog("task-1"));
    await waitFor(() => expect(result.current.status).toBe("success"));
    expect(result.current.groups).toEqual([{ session_id: "s1", entries: [] }]);
  });

  it("surfaces a catalog failure without throwing", async () => {
    catalog.mockRejectedValueOnce(new Error("boom"));
    const { result } = renderHook(() => useTaskDocumentsCatalog("task-1"));
    await waitFor(() => expect(result.current.status).toBe("error"));
    expect(result.current.groups).toEqual([]);
  });
});

describe("useTaskDocumentReview", () => {
  it("loads detail and the first revision page independently", async () => {
    detail.mockResolvedValueOnce({
      key: "spec",
      title: "Spec",
      type: "SPEC",
      is_plan: false,
      content: "hello",
      latest_revision_number: 2,
      updated_at: "2026-01-01T00:00:00Z",
    });
    revisions.mockResolvedValueOnce({
      revisions: [{ id: "r2", revision_number: 2 }],
      limit: 20,
    } as Awaited<ReturnType<typeof listTaskDocumentRevisions>>);
    const { result } = renderHook(() => useTaskDocumentReview("task-1", "spec"));
    await waitFor(() => expect(result.current.status).toBe("success"));
    expect(result.current.detail?.title).toBe("Spec");
    expect(result.current.revisions).toHaveLength(1);
    expect(revisions).toHaveBeenCalledWith("task-1", "spec", { limit: 20 });
  });

  it("pages backward from the oldest loaded revision", async () => {
    detail.mockResolvedValueOnce({ key: "spec", title: "Spec" } as never);
    revisions
      .mockResolvedValueOnce({
        revisions: [
          { id: "r3", revision_number: 3 },
          { id: "r2", revision_number: 2 },
        ],
        limit: 20,
      } as never)
      .mockResolvedValueOnce({ revisions: [{ id: "r1", revision_number: 1 }], limit: 20 } as never);
    const { result } = renderHook(() => useTaskDocumentReview("task-1", "spec"));
    await waitFor(() => expect(result.current.revisionsStatus).toBe("success"));
    expect(result.current.revisions).toHaveLength(2);
    result.current.loadMoreRevisions();
    await waitFor(() =>
      expect(revisions).toHaveBeenLastCalledWith("task-1", "spec", {
        limit: 20,
        beforeRevision: 2,
      }),
    );
  });

  it("does not fetch without a document key", () => {
    renderHook(() => useTaskDocumentReview("task-1", null));
    expect(detail).not.toHaveBeenCalled();
    expect(revisions).not.toHaveBeenCalled();
  });
});
