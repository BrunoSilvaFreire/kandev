"use client";

import { IconMessageCircle } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
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
          <ul className="divide-y rounded-lg border">
            {browse.items.map((item) => (
              <QuickChatBrowseRow
                key={item.sessionId}
                item={item}
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
