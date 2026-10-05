"use client";

import { useCallback, useState } from "react";
import type { RemoveSessionOptions } from "@/hooks/domains/session/use-session-actions";

type SetConfirmDelete = (open: boolean) => void;
type HandleDelete = (options?: RemoveSessionOptions) => Promise<boolean>;

/**
 * Delete confirmation state for a session tab's context menu.
 *
 * Closing a tab no longer deletes its session (`use-tab-context-actions` owns
 * close), so this hook only drives the explicit Delete action: the confirmation
 * popover reports its open state and, on confirm, runs the delete with the
 * default toast feedback.
 */
export function useSessionTabDelete(
  setConfirmDelete: SetConfirmDelete,
  handleDelete: HandleDelete,
) {
  const [isDeleting, setIsDeleting] = useState(false);

  const handleDeleteDialogOpenChange = useCallback(
    (open: boolean) => {
      setConfirmDelete(open);
    },
    [setConfirmDelete],
  );

  const handleConfirmDelete = useCallback(async () => {
    setIsDeleting(true);
    try {
      await handleDelete({ feedback: "toast" });
    } finally {
      setIsDeleting(false);
    }
  }, [handleDelete]);

  const handleMenuDelete = useCallback(() => {
    setConfirmDelete(true);
  }, [setConfirmDelete]);

  return { handleDeleteDialogOpenChange, handleConfirmDelete, handleMenuDelete, isDeleting };
}
