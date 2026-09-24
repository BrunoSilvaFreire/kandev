"use client";

import { useTranslation } from "react-i18next";
import type { SessionUsageInspector } from "@/hooks/domains/session/use-session-usage-inspector";
import type { PromptUsageEntry } from "@/lib/state/slices/session-runtime/types";
import { formatDollars } from "@/lib/utils";
import { cacheStatusLabelKey, formatHitRatio, formatNumber } from "@/lib/usage/format";
import { hitRatio } from "@/lib/usage/efficiency";

function SectionTitle({ children }: { children: string }) {
  return (
    <span className="text-[10px] font-medium uppercase text-muted-foreground">{children}</span>
  );
}

function ColumnHeader({ taskLabel }: { taskLabel: string }) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center justify-end gap-3 text-[10px] uppercase text-muted-foreground">
      <span className="w-16 text-right">{t("task:usageInspectorSession")}</span>
      <span className="w-16 text-right">{taskLabel}</span>
    </div>
  );
}

function UsageRow({
  label,
  session,
  task,
  sessionNote,
  taskNote,
}: {
  label?: string;
  session: string;
  task: string;
  sessionNote?: string | null;
  taskNote?: string | null;
}) {
  return (
    <div className="flex items-start justify-between gap-3">
      <span className="text-[11px] text-muted-foreground">{label}</span>
      <span className="flex gap-3">
        <span className="w-20 text-right">
          <span className="tabular-nums text-foreground">{session}</span>
          {sessionNote ? (
            <span className="block text-[9px] text-muted-foreground/80">{sessionNote}</span>
          ) : null}
        </span>
        <span className="w-20 text-right">
          <span className="tabular-nums text-foreground">{task}</span>
          {taskNote ? (
            <span className="block text-[9px] text-muted-foreground/80">{taskNote}</span>
          ) : null}
        </span>
      </span>
    </div>
  );
}

/** Cost string with a `~` prefix when any contributing event was estimated. */
function costValue(costSubcents: number, estimated: boolean): string {
  return `${estimated ? "~" : ""}${formatDollars(costSubcents)}`;
}

/** Last-prompt hit ratio using the same "not reported" rule as the session. */
function lastPromptHitRatio(lastPrompt: PromptUsageEntry | undefined): number | null {
  if (!lastPrompt) return null;
  const read = lastPrompt.cachedReadTokens ?? 0;
  const write = lastPrompt.cachedWriteTokens ?? 0;
  const cacheTokens = read + write;
  if (cacheTokens === 0) return null;
  const denominator = lastPrompt.inputTokens + cacheTokens;
  if (denominator === 0) return null;
  return read / denominator;
}

function CacheSection({ inspector }: { inspector: SessionUsageInspector }) {
  const { t } = useTranslation();
  const { session, lastPrompt, status, expiresAt } = inspector;
  const ratio = session ? formatHitRatio(hitRatio(session)) : null;
  const lastPromptRatio = formatHitRatio(lastPromptHitRatio(lastPrompt));

  return (
    <div className="space-y-1 border-t border-border pt-2" data-testid="usage-inspector-cache">
      <div className="flex items-baseline justify-between gap-6">
        <SectionTitle>{t("task:usageInspectorCacheTitle")}</SectionTitle>
        <span className="text-[11px] font-medium text-foreground">
          {t(cacheStatusLabelKey(status))}
        </span>
      </div>
      <p className="text-[10px] text-muted-foreground">{t("task:usageInspectorEstimateNote")}</p>
      {expiresAt !== null ? (
        <div className="flex items-center justify-between gap-3">
          <span className="text-[11px] text-muted-foreground">
            {t("task:usageInspectorExpiresAt")}
          </span>
          <span className="tabular-nums text-foreground">
            {new Date(expiresAt).toLocaleString()}
          </span>
        </div>
      ) : null}
      <div className="flex items-center justify-between gap-3">
        <span className="text-[11px] text-muted-foreground">
          {t("task:usageInspectorHitRatio")}
        </span>
        <span className="tabular-nums text-foreground">
          {ratio ?? t("task:usageInspectorHitRatioNotReported")}
        </span>
      </div>
      {lastPrompt ? (
        <div className="flex items-center justify-between gap-3">
          <span className="text-[11px] text-muted-foreground">
            {t("task:usageInspectorLastPrompt")}
          </span>
          <span className="tabular-nums text-foreground">
            {lastPromptRatio ?? t("task:usageInspectorHitRatioNotReported")}
          </span>
        </div>
      ) : null}
    </div>
  );
}

function TokensAndCost({ inspector }: { inspector: SessionUsageInspector }) {
  const { t } = useTranslation();
  const { session, task } = inspector;
  if (!session || !task) return null;
  const sessionIncomplete = session.output_tokens_complete
    ? null
    : t("task:usageInspectorIncomplete");
  const taskIncomplete = task.output_tokens_complete ? null : t("task:usageInspectorIncomplete");
  const costNote = (totals: typeof session) =>
    [
      totals.estimated_event_count > 0 ? t("task:usageInspectorEstimated") : null,
      totals.unpriced_event_count > 0
        ? t("task:usageInspectorUnpriced", { count: totals.unpriced_event_count })
        : null,
    ]
      .filter(Boolean)
      .join(", ") || null;

  return (
    <>
      <div className="space-y-1 border-t border-border pt-2" data-testid="usage-inspector-tokens">
        <div className="flex items-baseline justify-between gap-6">
          <SectionTitle>{t("task:usageInspectorTokensTitle")}</SectionTitle>
          <ColumnHeader taskLabel={t("task:usageInspectorTask")} />
        </div>
        <UsageRow
          label={t("task:usageInspectorInput")}
          session={formatNumber(session.tokens_in)}
          task={formatNumber(task.tokens_in)}
        />
        <UsageRow
          label={t("task:usageInspectorCachedRead")}
          session={formatNumber(session.tokens_cached_read)}
          task={formatNumber(task.tokens_cached_read)}
        />
        <UsageRow
          label={t("task:usageInspectorCachedWrite")}
          session={formatNumber(session.tokens_cached_write)}
          task={formatNumber(task.tokens_cached_write)}
        />
        <UsageRow
          label={t("task:usageInspectorOutput")}
          session={formatNumber(session.tokens_out)}
          task={formatNumber(task.tokens_out)}
          sessionNote={sessionIncomplete}
          taskNote={taskIncomplete}
        />
        <UsageRow
          label={t("task:usageInspectorThought")}
          session={formatNumber(session.tokens_thought)}
          task={formatNumber(task.tokens_thought)}
        />
        <UsageRow
          label={t("task:usageInspectorTotal")}
          session={formatNumber(session.tokens_total)}
          task={formatNumber(task.tokens_total)}
        />
      </div>
      <div className="space-y-1 border-t border-border pt-2" data-testid="usage-inspector-cost">
        <div className="flex items-baseline justify-between gap-6">
          <SectionTitle>{t("task:usageInspectorCostTitle")}</SectionTitle>
          <ColumnHeader taskLabel={t("task:usageInspectorTask")} />
        </div>
        <UsageRow
          session={costValue(session.cost_subcents, session.estimated_event_count > 0)}
          task={costValue(task.cost_subcents, task.estimated_event_count > 0)}
          sessionNote={costNote(session)}
          taskNote={costNote(task)}
        />
      </div>
    </>
  );
}

function FlagsSection({ inspector }: { inspector: SessionUsageInspector }) {
  const { t } = useTranslation();
  const { flags } = inspector;
  if (!flags) return null;
  const raised = [
    flags.lowHitRatio ? t("task:usageInspectorFlagLowHitRatio") : null,
    flags.highCost ? t("task:usageInspectorFlagHighCost") : null,
    flags.cacheLikelyExpired ? t("task:usageInspectorFlagCacheExpired") : null,
  ].filter((value): value is string => value !== null);
  if (raised.length === 0) return null;
  return (
    <div className="space-y-1 border-t border-border pt-2" data-testid="usage-inspector-flags">
      {raised.map((flag) => (
        <p key={flag} className="text-[11px] text-amber-500">
          {flag}
        </p>
      ))}
    </div>
  );
}

export function TokenUsageInspector({ inspector }: { inspector: SessionUsageInspector }) {
  const { t } = useTranslation();
  const { session, loading, error } = inspector;
  return (
    <div className="space-y-2" data-testid="usage-inspector">
      {error ? <p className="text-[11px] text-destructive">{error}</p> : null}
      {loading && !session ? (
        <p className="text-[11px] text-muted-foreground">{t("common:loading")}</p>
      ) : null}
      <CacheSection inspector={inspector} />
      {session && session.event_count === 0 ? (
        <p
          className="border-t border-border pt-2 text-[11px] text-muted-foreground"
          data-testid="usage-inspector-no-usage"
        >
          {t("task:usageInspectorNoUsage")}
        </p>
      ) : (
        <TokensAndCost inspector={inspector} />
      )}
      <FlagsSection inspector={inspector} />
    </div>
  );
}
