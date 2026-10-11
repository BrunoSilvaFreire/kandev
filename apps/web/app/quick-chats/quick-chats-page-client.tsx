"use client";

import { IconMessageCircle } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { useMemo } from "react";
import { useAppStore } from "@/components/state-provider";
import { useRepositories } from "@/hooks/domains/workspace/use-repositories";
import type { QuickChatBrowseOrder } from "@/lib/quick-chat/browse";
import { useTranslation } from "react-i18next";
import { PageShell } from "@/components/page-shell";
import { QuickChatDeleteDialog } from "@/components/quick-chat/quick-chat-delete-dialog";
import { useRouter } from "@/lib/routing/client-router";
import { QuickChatRenameDialog } from "./quick-chat-rename-dialog";
import { QuickChatBrowseRow } from "./quick-chat-browse-row";
import { useQuickChatsBrowse } from "./use-quick-chats-browse";

/**
 * Browse page for restorable Quick Chats. The panel remains the fast path; this
 * page is the durable way to find one after its tab was closed.
 */
export function QuickChatsPageClient() {
  const { t } = useTranslation();
  const router = useRouter();
  const browse = useQuickChatsBrowse();
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const { repositories } = useRepositories(workspaceId, true);
  const repositoryNames = useMemo(
    () => new Map(repositories.map((repository) => [repository.id, repository.name])),
    [repositories],
  );

  return (
    <PageShell
      title={t("sidebar:quickChats")}
      icon={<IconMessageCircle className="h-4 w-4" />}
      topbarTestId="quick-chats-topbar"
    >
      <div className="mx-auto w-full max-w-4xl px-4 py-6 sm:px-6">
        {browse.status === "loading" && (
          <p role="status" className="text-sm text-muted-foreground">
            {t("common:loading")}
          </p>
        )}
        {browse.status === "error" && (
          <div className="flex flex-col items-start gap-3">
            <p role="alert" className="text-sm text-destructive">
              {t("chat:quickChatsLoadFailed")}
            </p>
            <Button
              variant="outline"
              className="cursor-pointer"
              onClick={() => void browse.reload()}
            >
              {t("common:retry")}
            </Button>
          </div>
        )}
        {browse.status === "ready" && browse.items.length === 0 && (
          <div className="rounded-lg border border-dashed p-8 text-center">
            <p className="text-sm font-medium">{t("chat:quickChatsEmpty")}</p>
            <p className="mt-1 text-sm text-muted-foreground">{t("chat:quickChatsEmptyHint")}</p>
          </div>
        )}
        {browse.status === "ready" && browse.items.length > 0 && (
          <div className="mb-3 flex items-center justify-end gap-2">
            <span className="text-xs text-muted-foreground" id="quick-chats-order-label">
              {t("chat:quickChatsOrderLabel")}
            </span>
            <Select
              value={browse.order}
              onValueChange={(value) => browse.setOrder(value as QuickChatBrowseOrder)}
            >
              <SelectTrigger
                className="w-52 text-xs max-sm:min-h-11"
                aria-labelledby="quick-chats-order-label"
                data-testid="quick-chats-order"
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="recent" className="text-xs max-sm:min-h-11">
                  {t("chat:quickChatsOrderRecent")}
                </SelectItem>
                <SelectItem value="oldest" className="text-xs max-sm:min-h-11">
                  {t("chat:quickChatsOrderOldest")}
                </SelectItem>
              </SelectContent>
            </Select>
          </div>
        )}
        {browse.status === "ready" && browse.items.length > 0 && (
          <ul className="divide-y rounded-lg border" data-testid="quick-chats-list">
            {browse.items.map((item) => (
              <QuickChatBrowseRow
                key={item.sessionId}
                item={item}
                repositoryNames={repositoryNames}
                onOpen={() => router.push(`/quick-chats/${item.taskId}`)}
                onOpenInPanel={() => browse.handleOpenInPanel(item)}
                onRename={() => browse.setSessionToRename(item)}
                onDelete={() => browse.setSessionToDelete(item.sessionId)}
              />
            ))}
          </ul>
        )}
      </div>
      <QuickChatDeleteDialog
        sessionToDelete={browse.sessionToDelete}
        onOpenChange={(open) => !open && browse.setSessionToDelete(null)}
        onConfirm={() => void browse.handleConfirmDelete()}
      />
      <QuickChatRenameDialog
        open={browse.sessionToRename !== null}
        initialName={browse.sessionToRename?.name ?? ""}
        onOpenChange={(open) => !open && browse.setSessionToRename(null)}
        onSubmit={(name) => void browse.handleRename(name)}
      />
    </PageShell>
  );
}
