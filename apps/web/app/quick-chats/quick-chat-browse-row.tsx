"use client";

import { IconChecklist, IconDots, IconFolder, IconLayoutSidebar } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import TaskLink from "@/components/routing/task-link";
import { useAppStore } from "@/components/state-provider";
import { formatDateTime, formatRelativeCompact } from "@/lib/i18n/formats";
import { quickChatBrowseTitle, type QuickChatBrowseItem } from "@/lib/quick-chat/browse";

export type { QuickChatBrowseItem };

function useAgentLabel(agentProfileId: string | undefined): string | null {
  return useAppStore((state) => {
    if (!agentProfileId) return null;
    return (
      state.agentProfiles.items.find((profile) => profile.id === agentProfileId)?.label ?? null
    );
  });
}

/** Relative age plus the absolute time, readable without hover. */
function ActivityText({ item }: { item: QuickChatBrowseItem }) {
  const { t } = useTranslation();
  if (!item.lastActivityAt) return null;
  const absolute = formatDateTime(item.lastActivityAt);
  const text = item.hasMessages
    ? t("chat:quickChatLastActivity", {
        relative: formatRelativeCompact(item.lastActivityAt),
        date: absolute,
      })
    : t("chat:quickChatNoMessagesYet", { date: absolute });
  return (
    <time dateTime={item.lastActivityAt} data-testid={`quick-chat-activity-${item.sessionId}`}>
      {text}
    </time>
  );
}

/**
 * Links this chat can prove: repositories attached to it and tasks it created.
 * A repository the workspace no longer has resolves to no name and is skipped.
 */
function BrowseBadges({
  item,
  repositoryNames,
}: {
  item: QuickChatBrowseItem;
  repositoryNames: ReadonlyMap<string, string>;
}) {
  const { t } = useTranslation();
  const repositories = item.repositoryIds.flatMap((id) => {
    const name = repositoryNames.get(id);
    return name ? [{ id, name }] : [];
  });
  if (repositories.length === 0 && item.tasks.length === 0) return null;
  return (
    <div className="mt-1 flex flex-wrap items-center gap-1" data-testid="quick-chat-badges">
      {repositories.map((repository) => (
        <Badge
          key={repository.id}
          variant="outline"
          className="max-w-48 gap-1 font-normal"
          title={t("chat:quickChatLinkedRepository", { name: repository.name })}
          data-testid={`quick-chat-repository-badge-${repository.id}`}
        >
          <IconFolder className="h-3 w-3 shrink-0" aria-hidden />
          <span className="sr-only">{t("chat:quickChatLinkedRepositoryPrefix")}</span>
          <span className="truncate">{repository.name}</span>
        </Badge>
      ))}
      {item.tasks.map((task) => (
        <Badge
          key={task.id}
          asChild
          variant="secondary"
          className="max-w-48 cursor-pointer gap-1 font-normal max-sm:min-h-11"
        >
          <TaskLink
            taskId={task.id}
            title={t("chat:quickChatCreatedTask", { title: task.title })}
            data-testid={`quick-chat-task-badge-${task.id}`}
          >
            <IconChecklist className="h-3 w-3 shrink-0" aria-hidden />
            <span className="sr-only">{t("chat:quickChatCreatedTaskPrefix")}</span>
            <span className="truncate">{task.title}</span>
          </TaskLink>
        </Badge>
      ))}
      {item.moreTasks > 0 && (
        <span className="text-xs text-muted-foreground" data-testid="quick-chat-more-tasks">
          {t("chat:quickChatMoreTasks", { count: item.moreTasks })}
        </span>
      )}
    </div>
  );
}

export function QuickChatBrowseRow({
  item,
  repositoryNames,
  onOpen,
  onOpenInPanel,
  onRename,
  onDelete,
}: {
  item: QuickChatBrowseItem;
  repositoryNames: ReadonlyMap<string, string>;
  onOpen: () => void;
  onOpenInPanel: () => void;
  onRename: () => void;
  onDelete: () => void;
}) {
  const { t } = useTranslation();
  const agentLabel = useAgentLabel(item.agentProfileId);
  const title = quickChatBrowseTitle(item, t("chat:untitledQuickChat"));

  return (
    <li className="flex items-center gap-2 px-3 py-2.5 sm:px-4" data-testid="quick-chat-browse-row">
      <div className="min-w-0 flex-1">
        <button
          type="button"
          onClick={onOpen}
          className="block w-full min-w-0 cursor-pointer text-left"
          data-testid={`quick-chat-row-${item.sessionId}`}
        >
          <span className="block truncate text-sm font-medium" data-testid="quick-chat-row-title">
            {title}
          </span>
          <span className="mt-0.5 block truncate text-xs text-muted-foreground">
            {agentLabel}
            {agentLabel && item.lastActivityAt ? " · " : ""}
            <ActivityText item={item} />
          </span>
        </button>
        <BrowseBadges item={item} repositoryNames={repositoryNames} />
      </div>
      <Button
        variant="ghost"
        size="sm"
        className="hidden cursor-pointer sm:inline-flex"
        onClick={onOpenInPanel}
      >
        <IconLayoutSidebar className="mr-1.5 h-4 w-4" aria-hidden />
        {t("chat:openInPanel")}
      </Button>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            variant="ghost"
            size="sm"
            className="h-9 w-9 shrink-0 cursor-pointer p-0"
            aria-label={t("chat:quickChatRowActions", { name: title })}
          >
            <IconDots className="h-4 w-4" aria-hidden />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-48">
          <DropdownMenuItem className="min-h-11 cursor-pointer sm:min-h-7" onSelect={onOpen}>
            {t("common:open")}
          </DropdownMenuItem>
          <DropdownMenuItem className="min-h-11 cursor-pointer sm:min-h-7" onSelect={onOpenInPanel}>
            {t("chat:openInPanel")}
          </DropdownMenuItem>
          <DropdownMenuItem className="min-h-11 cursor-pointer sm:min-h-7" onSelect={onRename}>
            {t("common:rename")}
          </DropdownMenuItem>
          <DropdownMenuItem
            className="min-h-11 cursor-pointer text-destructive sm:min-h-7"
            onSelect={onDelete}
          >
            {t("common:delete")}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </li>
  );
}
