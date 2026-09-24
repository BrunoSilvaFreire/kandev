import { beforeEach, describe, expect, it, vi } from "vitest";

const mockFetchJson = vi.hoisted(() => vi.fn());
vi.mock("../client", () => ({
  fetchJson: mockFetchJson,
  fetchJsonWithRetry: vi.fn(),
}));

import {
  createOrUpdateDocument,
  deleteDocument,
  getDocument,
  listDocuments,
} from "./office-extended-api";

const TASK_ID = "task-1";

beforeEach(() => {
  mockFetchJson.mockReset();
  mockFetchJson.mockResolvedValue({ documents: [] });
});

// Task documents are a task record, so the shared client must target the
// task-scoped route (available with features.office off), not the Office group.
describe("task document API paths", () => {
  it("lists documents on the task-scoped route", () => {
    void listDocuments(TASK_ID);

    expect(mockFetchJson).toHaveBeenCalledWith("/api/v1/tasks/task-1/documents", undefined);
  });

  it("gets a document on the task-scoped route with an encoded key", () => {
    void getDocument(TASK_ID, "spike");

    expect(mockFetchJson).toHaveBeenCalledWith("/api/v1/tasks/task-1/documents/spike", undefined);
  });

  it("creates or updates a document on the task-scoped route", () => {
    void createOrUpdateDocument(TASK_ID, "spike", { type: "spike", content: "body" });

    const [url, options] = mockFetchJson.mock.calls[0];
    expect(url).toBe("/api/v1/tasks/task-1/documents/spike");
    expect(options.init.method).toBe("PUT");
  });

  it("deletes a document on the task-scoped route", () => {
    void deleteDocument(TASK_ID, "spike");

    const [url, options] = mockFetchJson.mock.calls[0];
    expect(url).toBe("/api/v1/tasks/task-1/documents/spike");
    expect(options.init.method).toBe("DELETE");
  });
});
