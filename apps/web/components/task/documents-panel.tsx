"use client";

import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { IconFileText, IconLoader2 } from "@tabler/icons-react";
import { useAppStore } from "@/components/state-provider";
import { useTaskDocumentsCatalog } from "@/hooks/domains/task/use-task-documents";
import type { TaskDocumentCatalogGroup } from "@/lib/types/task-document";

type DocumentsPanelProps = {
  taskId: string | null;
  onOpenDocument: (key: string) => void;
};

/** Resolve a catalog group's session label from live store data, falling back
 *  to the persisted id and then to the Unknown/legacy group label. */
function useGroupLabel(group: TaskDocumentCatalogGroup): string {
  const { t } = useTranslation();
  const sessionName = useAppStore((state) =>
    group.session_id ? (state.taskSessions.items[group.session_id]?.name ?? "") : "",
  );
  if (!group.session_id) return t("task:documentsGroupUnassigned");
  return sessionName || group.session_id;
}

function DocumentGroup({
  group,
  onOpenDocument,
}: {
  group: TaskDocumentCatalogGroup;
  onOpenDocument: (key: string) => void;
}) {
  const label = useGroupLabel(group);
  return (
    <div className="space-y-0.5" data-testid="document-group">
      <h3 className="truncate px-1 text-xs font-medium text-muted-foreground">{label}</h3>
      {group.entries.map((entry) => (
        <button
          key={entry.key}
          type="button"
          className="flex w-full min-w-0 cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm hover:bg-accent/50 max-md:min-h-11"
          onClick={() => onOpenDocument(entry.key)}
          data-testid={`document-row-${entry.key}`}
        >
          <IconFileText className="h-4 w-4 shrink-0 text-muted-foreground" />
          <span className="min-w-0 flex-1 truncate">{entry.title || entry.key}</span>
          <span className="shrink-0 text-xs text-muted-foreground">
            {entry.latest_revision_number > 0
              ? `rev ${entry.latest_revision_number}`
              : entry.type.toUpperCase()}
          </span>
        </button>
      ))}
    </div>
  );
}

/** Metadata-only task document catalog. Entries open the Review surface; the
 *  authoritative Plan is included by the server and opens the editable plan. */
export function DocumentsPanel({ taskId, onOpenDocument }: DocumentsPanelProps) {
  const { t } = useTranslation();
  const { groups, status, reload } = useTaskDocumentsCatalog(taskId);

  if (status === "loading") {
    return (
      <div className="flex h-full items-center justify-center text-muted-foreground">
        <IconLoader2 className="mr-2 h-4 w-4 animate-spin" />
        <span className="text-sm">{t("task:loading")}</span>
      </div>
    );
  }

  if (status === "error") {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-3 text-muted-foreground">
        <span className="text-sm">{t("task:failedToLoadDocuments")}</span>
        <Button variant="outline" size="sm" className="cursor-pointer" onClick={reload}>
          {t("task:retry")}
        </Button>
      </div>
    );
  }

  if (groups.length === 0) {
    return (
      <div className="flex h-full items-center justify-center text-muted-foreground">
        <span className="text-sm">{t("task:noDocumentsYet")}</span>
      </div>
    );
  }

  return (
    <div className="h-full overflow-y-auto p-2" data-testid="documents-panel">
      <div className="space-y-3">
        {groups.map((group) => (
          <DocumentGroup
            key={group.session_id ?? "__unassigned__"}
            group={group}
            onOpenDocument={onOpenDocument}
          />
        ))}
      </div>
    </div>
  );
}
