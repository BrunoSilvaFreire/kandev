import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { useSessionTabDelete } from "./use-session-tab-delete";

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((nextResolve) => {
    resolve = nextResolve;
  });
  return { promise, resolve };
}

describe("useSessionTabDelete", () => {
  it("opens the confirmation from the context menu and deletes with toast feedback", async () => {
    const setConfirmDelete = vi.fn();
    const handleDelete = vi.fn().mockResolvedValue(true);
    const { result } = renderHook(() => useSessionTabDelete(setConfirmDelete, handleDelete));

    act(() => result.current.handleMenuDelete());
    expect(setConfirmDelete).toHaveBeenCalledWith(true);

    await act(async () => {
      await result.current.handleConfirmDelete();
    });

    expect(handleDelete).toHaveBeenCalledWith({ feedback: "toast" });
    expect(result.current.isDeleting).toBe(false);
  });

  it("tracks the deleting state until the request settles", async () => {
    const setConfirmDelete = vi.fn();
    const pending = deferred<boolean>();
    const handleDelete = vi.fn(() => pending.promise);
    const { result } = renderHook(() => useSessionTabDelete(setConfirmDelete, handleDelete));

    let deletePromise!: Promise<void>;
    act(() => {
      deletePromise = result.current.handleConfirmDelete();
    });
    expect(result.current.isDeleting).toBe(true);

    await act(async () => {
      pending.resolve(true);
      await deletePromise;
    });
    expect(result.current.isDeleting).toBe(false);
  });

  it("clears the deleting state when the request fails", async () => {
    const setConfirmDelete = vi.fn();
    const handleDelete = vi.fn().mockResolvedValue(false);
    const { result } = renderHook(() => useSessionTabDelete(setConfirmDelete, handleDelete));

    await act(async () => {
      await result.current.handleConfirmDelete();
    });

    expect(result.current.isDeleting).toBe(false);
  });
});
