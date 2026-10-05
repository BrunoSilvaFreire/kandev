"use client";

import { IconDots, IconLayoutSidebar } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { useAppStore } from "@/components/state-provider";
import { formatRelativeCompact } from "@/lib/i18n/formats";

export type QuickChatBrowseItem = {
  sessionId: string;
  taskId: string;
  workspaceId: string;
  kind: "chat" | "config";
  name: string;
  agentProfileId?: string;
  lastActivityAt: string | null;
};

function useAgentLabel(agentProfileId: string | undefined): string | null {
  return useAppStore((state) => {
    if (!agentProfileId) return null;
    return (
      state.agentProfiles.items.find((profile) => profile.id === agentProfileId)?.label ?? null
    );
  });
}

export function QuickChatBrowseRow({
  item,
  onOpen,
  onOpenInPanel,
  onRename,
  onDelete,
}: {
  item: QuickChatBrowseItem;
  onOpen: () => void;
  onOpenInPanel: () => void;
  onRename: () => void;
  onDelete: () => void;
}) {
  const { t } = useTranslation();
  const agentLabel = useAgentLabel(item.agentProfileId);
  const title = item.name || t("chat:untitledQuickChat");
  const lastActivity = item.lastActivityAt ? formatRelativeCompact(item.lastActivityAt) : "";

  return (
    <li className="flex items-center gap-2 px-3 py-2.5 sm:px-4">
      <button
        type="button"
        onClick={onOpen}
        className="min-w-0 flex-1 cursor-pointer text-left"
        data-testid={`quick-chat-row-${item.sessionId}`}
      >
        <span className="block truncate text-sm font-medium">{title}</span>
        <span className="mt-0.5 block truncate text-xs text-muted-foreground">
          {[agentLabel, lastActivity].filter(Boolean).join(" · ")}
        </span>
      </button>
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
