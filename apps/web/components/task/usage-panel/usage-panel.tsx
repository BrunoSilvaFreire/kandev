"use client";

import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import { useAppStore } from "@/components/state-provider";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useTaskUsageBreakdown } from "@/hooks/domains/session/use-task-usage-breakdown";
import type { UsageView } from "@/lib/usage/breakdown";
import { cn } from "@/lib/utils";
import { UsageTotalsCard } from "./usage-totals-card";
import { UsageTable } from "./usage-table";
import { UsageCards } from "./usage-cards";
import { buildUsageDisplayRows, type UsageDisplayRow } from "./usage-panel-rows";

function UsageBody({
  rows,
  isMobile,
  t,
}: {
  rows: UsageDisplayRow[];
  isMobile: boolean;
  t: TFunction;
}) {
  if (rows.length === 0) {
    return (
      <div className="text-xs text-muted-foreground" data-testid="usage-panel-no-rows">
        {t("task:usagePanelNoUsage")}
      </div>
    );
  }
  return isMobile ? <UsageCards rows={rows} /> : <UsageTable rows={rows} />;
}

const VIEWS: UsageView[] = ["agent", "model", "session"];
const VIEW_LABEL_KEY: Record<UsageView, string> = {
  agent: "task:usagePanelViewAgent",
  model: "task:usagePanelViewModel",
  session: "task:usagePanelViewSession",
};

/**
 * The dockable Usage panel: a task-wide header card plus per-agent, per-model
 * and per-session roll-ups of the ledger breakdown. The pinned popup on the
 * token indicator stays the quick view; this is the detailed one.
 */
export function UsagePanel({ taskId }: { taskId: string | null }) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const [view, setView] = useState<UsageView>("agent");
  const { task, groups, lastPromptBySession, loading, error, now } = useTaskUsageBreakdown(taskId, {
    enabled: Boolean(taskId),
  });
  const sessions = useAppStore((state) =>
    taskId ? state.taskSessionsByTask.itemsByTaskId[taskId] : undefined,
  );
  const profiles = useAppStore((state) => state.agentProfiles.items);
  const messagesBySession = useAppStore((state) => state.messages.bySession);

  const rows = useMemo(
    () =>
      buildUsageDisplayRows(
        { view, groups, sessions, profiles, lastPromptBySession, messagesBySession, now },
        t("task:usagePanelUnknown"),
      ),
    [view, groups, sessions, profiles, lastPromptBySession, messagesBySession, now, t],
  );

  if (!taskId) {
    return (
      <div className="p-4 text-sm text-muted-foreground" data-testid="usage-panel-empty">
        {t("task:usagePanelNoTask")}
      </div>
    );
  }

  return (
    <div
      className="flex h-full min-h-0 flex-col gap-2 overflow-y-auto p-3"
      data-testid="usage-panel"
    >
      <UsageTotalsCard totals={task} />
      <div className="flex gap-1" role="tablist" aria-label={t("task:panelUsage")}>
        {VIEWS.map((candidate) => (
          <button
            key={candidate}
            type="button"
            role="tab"
            aria-selected={view === candidate}
            data-testid={`usage-panel-view-${candidate}`}
            onClick={() => setView(candidate)}
            className={cn(
              "min-h-8 cursor-pointer rounded px-2 text-xs",
              view === candidate ? "bg-muted font-medium text-foreground" : "text-muted-foreground hover:bg-muted/60",
            )}
          >
            {t(VIEW_LABEL_KEY[candidate])}
          </button>
        ))}
      </div>
      {error ? (
        <div className="text-xs text-destructive" data-testid="usage-panel-error">
          {error}
        </div>
      ) : null}
      {loading && rows.length === 0 ? (
        <div className="text-xs text-muted-foreground">{t("common:loading")}</div>
      ) : null}
      <UsageBody rows={rows} isMobile={isMobile} t={t} />
    </div>
  );
}
