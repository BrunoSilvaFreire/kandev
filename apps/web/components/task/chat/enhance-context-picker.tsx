"use client";

import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Checkbox } from "@kandev/ui/checkbox";
import { ScrollArea } from "@kandev/ui/scroll-area";
import { useAppStore } from "@/components/state-provider";
import { getTaskDocument } from "@/lib/api/domains/task-document-api";
import { useTaskDocumentsCatalog } from "@/hooks/domains/task/use-task-documents";
import type { SelectedContextItem } from "@/lib/api/domains/utility-api";
import type { TaskDocumentSummary } from "@/lib/types/task-document";
import type { ContextItem } from "@/lib/types/context";

const MAX_ITEMS = 8;

type PickerOption = {
  id: string;
  label: string;
  /** Resolves the selected-context item, fetching document content lazily. */
  resolve: () => Promise<SelectedContextItem>;
};

function contextChipOption(item: ContextItem): PickerOption | null {
  if (item.kind === "document-comment") {
    return {
      id: `chip:${item.id}`,
      label: item.label,
      resolve: async () => ({
        kind: "document_selection",
        label: item.label,
        text: item.comments
          .map(
            (comment) =>
              `${comment.documentKey} (revision ${comment.revision}): ${comment.selectedText}\n${comment.text}`,
          )
          .join("\n\n"),
      }),
    };
  }
  if (item.kind === "plan-comment") {
    return {
      id: `chip:${item.id}`,
      label: item.label,
      resolve: async () => ({
        kind: "document_selection",
        label: item.label,
        text: item.comments
          .map((comment) => `${comment.selectedText}\n${comment.text}`)
          .join("\n\n"),
      }),
    };
  }
  if (item.kind === "agent-message-comment") {
    return {
      id: `chip:${item.id}`,
      label: item.label,
      resolve: async () => ({
        kind: "session",
        label: item.label,
        text: item.comments
          .map((comment) => `${comment.selectedText}\n${comment.text}`)
          .join("\n\n"),
      }),
    };
  }
  return null;
}

function useSessionOptions(taskId: string | null) {
  const sessions = useAppStore((state) =>
    taskId ? (state.taskSessionsByTask.itemsByTaskId[taskId] ?? []) : [],
  );
  const messagesBySession = useAppStore((state) => state.messages.bySession);
  return useMemo(
    () =>
      sessions.map((session) => ({
        id: session.id,
        label: session.name || session.id,
        text: (() => {
          const messages = messagesBySession[session.id] ?? [];
          const initialPrompt =
            messages.find((message) => message.author_type === "user")?.content ?? "";
          const lastAgent = [...messages]
            .reverse()
            .find((message) => message.author_type !== "user");
          return [initialPrompt, lastAgent?.content ?? ""].filter(Boolean).join("\n\n");
        })(),
      })),
    [messagesBySession, sessions],
  );
}

type PickerRow = { id: string; label: string };

function docOptionId(key: string): string {
  return `doc:${key}`;
}

function sessionOptionId(id: string): string {
  return `session:${id}`;
}

type SessionOption = { id: string; label: string; text: string };

async function resolveSelected(
  selected: Set<string>,
  chipOptions: PickerOption[],
  documentEntries: TaskDocumentSummary[],
  sessions: SessionOption[],
  taskId: string | null,
): Promise<SelectedContextItem[]> {
  const items: SelectedContextItem[] = [];
  for (const option of chipOptions) {
    if (selected.has(option.id)) items.push(await option.resolve());
  }
  for (const entry of documentEntries) {
    if (!selected.has(docOptionId(entry.key))) continue;
    try {
      const detail = await getTaskDocument(taskId ?? "", entry.key);
      items.push({ kind: "document", label: entry.title || entry.key, text: detail.content ?? "" });
    } catch {
      items.push({ kind: "document", label: entry.title || entry.key, text: "" });
    }
  }
  for (const session of sessions) {
    if (selected.has(sessionOptionId(session.id))) {
      items.push({ kind: "session", label: session.label, text: session.text });
    }
  }
  return items.slice(0, MAX_ITEMS);
}

function PickerGroup({
  title,
  rows,
  selected,
  onToggle,
}: {
  title: string;
  rows: PickerRow[];
  selected: Set<string>;
  onToggle: (id: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-1">
      <p className="px-1 text-[0.6875rem] font-medium uppercase text-muted-foreground">{title}</p>
      {rows.map((row) => (
        <label
          key={row.id}
          className="flex cursor-pointer items-center gap-2 rounded px-1 py-1 text-xs hover:bg-accent/40"
        >
          <Checkbox checked={selected.has(row.id)} onCheckedChange={() => onToggle(row.id)} />
          <span className="truncate">{row.label}</span>
        </label>
      ))}
      {rows.length === 0 && (
        <p className="px-1 text-[0.6875rem] text-muted-foreground">
          {t("task:enhanceContextEmpty")}
        </p>
      )}
    </div>
  );
}

/**
 * Explicit context picker for Enhance with AI. Nothing is selected by default
 * and there is no full-history option: the user chooses the current chat
 * context chips, task documents, and task sessions to include. Document bodies
 * are fetched only for the selected keys when the user confirms.
 */
export function EnhanceContextPicker({
  taskId,
  contextItems,
  documentKeyFilter,
  onConfirm,
}: {
  taskId: string | null;
  contextItems: ContextItem[];
  /** When set, only this document key is offered (per-document surface). */
  documentKeyFilter?: string;
  onConfirm: (items: SelectedContextItem[]) => void;
}) {
  const { t } = useTranslation();
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [busy, setBusy] = useState(false);
  const catalog = useTaskDocumentsCatalog(taskId);
  const sessions = useSessionOptions(taskId);

  const chipOptions = useMemo(
    () => contextItems.map(contextChipOption).filter((option): option is PickerOption => !!option),
    [contextItems],
  );
  const documentEntries = useMemo(() => {
    const entries = catalog.groups.flatMap((group) => group.entries);
    return documentKeyFilter ? entries.filter((entry) => entry.key === documentKeyFilter) : entries;
  }, [catalog.groups, documentKeyFilter]);

  const toggle = (id: string) => {
    setSelected((current) => {
      const next = new Set(current);
      if (next.has(id)) next.delete(id);
      else if (next.size < MAX_ITEMS) next.add(id);
      return next;
    });
  };

  const handleConfirm = async () => {
    setBusy(true);
    try {
      onConfirm(await resolveSelected(selected, chipOptions, documentEntries, sessions, taskId));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="w-80 space-y-3" data-testid="enhance-context-picker">
      <ScrollArea className="h-72">
        <div className="space-y-3 pr-2">
          <PickerGroup
            title={t("task:enhanceContextCurrent")}
            rows={chipOptions}
            selected={selected}
            onToggle={toggle}
          />
          <PickerGroup
            title={t("task:enhanceContextDocuments")}
            rows={documentEntries.map((entry) => ({
              id: docOptionId(entry.key),
              label: entry.title || entry.key,
            }))}
            selected={selected}
            onToggle={toggle}
          />
          <PickerGroup
            title={t("task:enhanceContextSessions")}
            rows={sessions.map((session) => ({
              id: sessionOptionId(session.id),
              label: session.label,
            }))}
            selected={selected}
            onToggle={toggle}
          />
        </div>
      </ScrollArea>
      <div className="flex items-center justify-between">
        <span className="text-[0.6875rem] text-muted-foreground">
          {t("task:enhanceContextSelected", { count: selected.size })}
        </span>
        <Button
          type="button"
          size="sm"
          className="cursor-pointer"
          disabled={busy}
          onClick={() => void handleConfirm()}
          data-testid="enhance-context-confirm"
        >
          {t("task:enhanceContextConfirm")}
        </Button>
      </div>
    </div>
  );
}
