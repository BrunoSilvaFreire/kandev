"use client";

import { useCallback, useEffect, useState, type RefObject } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@kandev/ui/tooltip";
import { PlanSelectionPopover } from "./plan-selection-popover";
import { useCommentsStore } from "@/lib/state/slices/comments";
import { generateUUID } from "@/lib/utils";

type DocumentSelection = {
  text: string;
  position: { x: number; y: number };
};

function isIgnoredSelectionTarget(target: EventTarget | null): boolean {
  if (!(target instanceof Element)) return false;
  return Boolean(
    target.closest(
      [
        "button",
        "a",
        "textarea",
        "input",
        "select",
        "[data-markdown-comment-popover]",
        "[data-markdown-preview-toolbar]",
      ].join(","),
    ),
  );
}

/** Capture a same-root text selection with a viewport anchor for the popover. */
function useDocumentTextSelection(rootRef: RefObject<HTMLDivElement | null>) {
  const [selection, setSelection] = useState<DocumentSelection | null>(null);

  useEffect(() => {
    const root = rootRef.current;
    if (!root) return;
    const resolve = (event: MouseEvent | TouchEvent) => {
      if (isIgnoredSelectionTarget(event.target)) return;
      const browserSelection = window.getSelection();
      const text = browserSelection?.toString().trim() ?? "";
      if (!browserSelection || browserSelection.isCollapsed || !text) {
        setSelection(null);
        return;
      }
      const range = browserSelection.getRangeAt(0);
      if (!root.contains(range.commonAncestorContainer)) {
        setSelection(null);
        return;
      }
      const rect = range.getBoundingClientRect();
      setSelection({ text, position: { x: rect.left + rect.width / 2, y: rect.bottom } });
    };
    root.addEventListener("mouseup", resolve);
    root.addEventListener("touchend", resolve);
    return () => {
      root.removeEventListener("mouseup", resolve);
      root.removeEventListener("touchend", resolve);
    };
  }, [rootRef]);

  const clear = useCallback(() => {
    setSelection(null);
    window.getSelection()?.removeAllRanges();
  }, []);

  return { selection, clear };
}

type DocumentSelectionCommentProps = {
  containerRef: RefObject<HTMLDivElement | null>;
  documentKey: string;
  revision: number;
  /** Owning chat session at selection time; unset disables the affordance. */
  sessionId: string | null;
};

/**
 * Anchors a document selection as pending chat context. It reuses the plan
 * selection composer but writes a DocumentComment to the shared session-scoped
 * comments store, so it is sent and cleared exactly like every other pending
 * chat comment. With no active session the affordance is disabled, not queued.
 */
export function DocumentSelectionComment({
  containerRef,
  documentKey,
  revision,
  sessionId,
}: DocumentSelectionCommentProps) {
  const { t } = useTranslation();
  const { selection, clear } = useDocumentTextSelection(containerRef);

  const handleAdd = useCallback(
    (text: string, selectedText: string) => {
      if (!sessionId) return false;
      useCommentsStore.getState().addComment({
        id: generateUUID(),
        sessionId,
        source: "document",
        documentKey,
        revision,
        selectedText,
        text,
        createdAt: new Date().toISOString(),
        status: "pending",
      });
      clear();
      return true;
    },
    [clear, documentKey, revision, sessionId],
  );

  if (!selection) return null;

  if (!sessionId) {
    return (
      <TooltipProvider>
        <Tooltip>
          <TooltipTrigger asChild>
            <span
              className="fixed z-[60] inline-flex"
              style={{ left: selection.position.x, top: selection.position.y }}
            >
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled
                data-testid="document-comment-disabled"
              >
                {t("task:documentCommentAction")}
              </Button>
            </span>
          </TooltipTrigger>
          <TooltipContent>{t("task:documentCommentNoActiveSession")}</TooltipContent>
        </Tooltip>
      </TooltipProvider>
    );
  }

  return (
    <PlanSelectionPopover
      selectedText={selection.text}
      position={selection.position}
      onAdd={handleAdd}
      onClose={clear}
      testId="document-selection-popover"
      inputTestId="document-selection-input"
      addButtonTestId="document-selection-add"
    />
  );
}
