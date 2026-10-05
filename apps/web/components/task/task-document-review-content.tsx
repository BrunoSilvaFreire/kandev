"use client";

import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { IconChevronDown, IconChevronRight, IconClock, IconLoader2 } from "@tabler/icons-react";
import { useAppStore } from "@/components/state-provider";
import { PanelBody, PanelRoot } from "./panel-primitives";
import { MarkdownPreviewRenderer } from "./markdown-preview-content";
import { DocumentSelectionComment } from "./document-selection-comment";
import { useTaskDocumentReview } from "@/hooks/domains/task/use-task-documents";
import { useDocumentSourceLabel } from "@/hooks/domains/task/use-document-source-label";
import { setPanelTitle } from "@/lib/layout/panel-portal-manager";
import { formatRelativeTime } from "@/lib/utils";
import type { TaskDocumentRevision } from "@/lib/types/task-document";

type Props = {
  panelId: string;
  taskId: string | null;
  documentKey: string;
};

/** Read-only Review surface for a non-plan task document. The Plan key keeps
 *  using the editable TaskPlanPanel; this component never edits or comments. */
export function TaskDocumentReviewContent({ panelId, taskId, documentKey }: Props) {
  const { t } = useTranslation();
  const review = useTaskDocumentReview(taskId, documentKey);
  const sourceLabel = useDocumentSourceLabel(
    review.detail?.source_session_id,
    review.detail?.source_workflow_step_id,
  );
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const bodyRef = useRef<HTMLDivElement>(null);
  const [revisionsOpen, setRevisionsOpen] = useState(false);

  useEffect(() => {
    if (review.detail) setPanelTitle(panelId, review.detail.title);
  }, [panelId, review.detail]);

  if (review.status === "loading" && !review.detail) {
    return (
      <div className="flex h-full items-center justify-center text-muted-foreground">
        <IconLoader2 className="mr-2 h-5 w-5 animate-spin" />
        <span className="text-sm">{t("task:loading")}</span>
      </div>
    );
  }

  if (review.status === "error" && !review.detail) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-3 text-muted-foreground">
        <span className="text-sm">{t("task:documentReviewLoadFailed")}</span>
        <Button variant="outline" size="sm" className="cursor-pointer" onClick={review.reload}>
          {t("task:retry")}
        </Button>
      </div>
    );
  }

  if (!review.detail) {
    return (
      <div className="flex h-full items-center justify-center text-muted-foreground">
        <span className="text-sm">{t("task:documentReviewEmpty")}</span>
      </div>
    );
  }

  return (
    <PanelRoot data-testid="task-document-review">
      <div className="flex flex-col gap-0.5 border-b border-border/80 px-3 py-2">
        <div className="flex items-center gap-2">
          <h2 className="truncate text-sm font-medium">{review.detail.title}</h2>
          <span className="text-xs text-muted-foreground">
            {t("task:revisionNumber", { number: review.detail.latest_revision_number })}
          </span>
        </div>
        <p className="truncate text-xs text-muted-foreground" data-testid="document-source-label">
          {sourceLabel ?? t("task:legacySource")}
        </p>
      </div>
      <PanelBody padding={false} scroll>
        <div
          ref={bodyRef}
          className="markdown-body max-w-3xl p-6"
          data-testid="task-document-review-body"
        >
          <MarkdownPreviewRenderer content={review.detail.content} taskId={taskId} />
        </div>
      </PanelBody>
      <DocumentSelectionComment
        containerRef={bodyRef}
        documentKey={documentKey}
        revision={review.detail.latest_revision_number}
        sessionId={activeSessionId ?? null}
      />
      <DocumentRevisionDisclosure
        open={revisionsOpen}
        onToggle={() => setRevisionsOpen((value) => !value)}
        revisions={review.revisions}
        hasMore={review.hasMoreRevisions}
        onLoadMore={review.loadMoreRevisions}
      />
    </PanelRoot>
  );
}

function DocumentRevisionDisclosure({
  open,
  onToggle,
  revisions,
  hasMore,
  onLoadMore,
}: {
  open: boolean;
  onToggle: () => void;
  revisions: TaskDocumentRevision[];
  hasMore: boolean;
  onLoadMore: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="shrink-0 border-t border-border/60" data-testid="document-revisions">
      <button
        type="button"
        className="flex w-full cursor-pointer items-center gap-1.5 px-3 py-2 text-xs text-muted-foreground hover:bg-accent/40"
        onClick={onToggle}
        aria-expanded={open}
        data-testid="document-revisions-toggle"
      >
        {open ? (
          <IconChevronDown className="h-3.5 w-3.5" />
        ) : (
          <IconChevronRight className="h-3.5 w-3.5" />
        )}
        <IconClock className="h-3.5 w-3.5" />
        {t("task:revisionHistory")}
      </button>
      {open && (
        <div className="max-h-56 overflow-y-auto px-3 pb-3">
          <ul className="space-y-1">
            {revisions.map((revision) => (
              <li
                key={revision.id}
                className="flex items-center justify-between gap-2 text-xs"
                data-testid="document-revision-row"
              >
                <span className="truncate font-medium">
                  {t("task:revisionNumber", { number: revision.revision_number })}
                </span>
                <span className="truncate text-muted-foreground">{revision.author_name}</span>
                <span className="shrink-0 text-muted-foreground">
                  {formatRelativeTime(revision.created_at)}
                </span>
              </li>
            ))}
          </ul>
          {revisions.length === 0 && (
            <p className="py-1 text-xs text-muted-foreground">{t("task:noRevisionsYet")}</p>
          )}
          {hasMore && (
            <Button
              variant="ghost"
              size="sm"
              className="mt-1 cursor-pointer text-xs"
              onClick={onLoadMore}
            >
              {t("task:loadMoreRevisions")}
            </Button>
          )}
        </div>
      )}
    </div>
  );
}
