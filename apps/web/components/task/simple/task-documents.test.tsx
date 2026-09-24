import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mockListDocuments = vi.hoisted(() => vi.fn());
const mockCreateOrUpdateDocument = vi.hoisted(() => vi.fn());
const mockDeleteDocument = vi.hoisted(() => vi.fn());

vi.mock("@/lib/api/domains/office-extended-api", () => ({
  listDocuments: mockListDocuments,
  createOrUpdateDocument: mockCreateOrUpdateDocument,
  deleteDocument: mockDeleteDocument,
}));

import { TaskDocuments } from "./task-documents";

const TASK_ID = "task-1";
const SPIKE_TITLE = "ACP step advancement spike";
const SPIKE_BODY = "Verified facts with file:line citations.";

function spikeDocument() {
  return {
    key: "spike",
    taskId: TASK_ID,
    type: "spike",
    title: SPIKE_TITLE,
    content: SPIKE_BODY,
    revision: 2,
    updatedAt: new Date().toISOString(),
    createdAt: new Date().toISOString(),
  };
}

beforeEach(() => {
  mockListDocuments.mockReset();
  mockCreateOrUpdateDocument.mockReset();
  mockDeleteDocument.mockReset();
});

afterEach(() => {
  cleanup();
});

describe("TaskDocuments", () => {
  it("renders a SPIKE badge for a spike document and hides its content until expanded", async () => {
    mockListDocuments.mockResolvedValue({ documents: [spikeDocument()] });

    render(<TaskDocuments taskId={TASK_ID} />);

    expect(await screen.findByText("SPIKE")).toBeTruthy();
    expect(screen.getByText(SPIKE_TITLE)).toBeTruthy();
    // The list projection is metadata only; the body appears on expansion.
    expect(screen.queryByText(SPIKE_BODY)).toBeNull();

    fireEvent.click(screen.getByText(SPIKE_TITLE));

    expect(await screen.findByText(SPIKE_BODY)).toBeTruthy();
  });

  it("renders the empty state when the task has no documents", async () => {
    mockListDocuments.mockResolvedValue({ documents: [] });

    render(<TaskDocuments taskId={TASK_ID} />);

    expect(await screen.findByText("No documents yet.")).toBeTruthy();
  });

  it("loads documents for the given task", async () => {
    mockListDocuments.mockResolvedValue({ documents: [] });

    render(<TaskDocuments taskId={TASK_ID} />);

    await screen.findByText("No documents yet.");
    expect(mockListDocuments).toHaveBeenCalledWith(TASK_ID);
  });

  it("renders a delete button with accessible label and deletes document on click", async () => {
    mockListDocuments.mockResolvedValue({ documents: [spikeDocument()] });
    mockDeleteDocument.mockResolvedValue({});

    render(<TaskDocuments taskId={TASK_ID} />);

    const deleteBtn = await screen.findByRole("button", { name: "Delete document" });
    expect(deleteBtn).toBeTruthy();

    fireEvent.click(deleteBtn);

    expect(mockDeleteDocument).toHaveBeenCalledWith(TASK_ID, "spike");
  });
});

