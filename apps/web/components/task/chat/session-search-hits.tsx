"use client";

import { useMemo } from "react";
import { cn } from "@/lib/utils";
import { AgentLogo } from "@/components/agent-logo";
import { useAppStore } from "@/components/state-provider";
import type { MessageSearchHit } from "@/lib/api/domains/session-api";
import { useTranslation } from "react-i18next";

type SessionSearchHitsProps = {
  hits: MessageSearchHit[];
  query: string;
  activeHitId: string | null;
  onSelect: (id: string) => void;
  isSearching: boolean;
  /** Display name for agent hits (e.g. the profile name). Falls back to "Agent". */
  agentLabel?: string | null;
  /** Agent registry slug (e.g. "claude-code") used to fetch the profile logo. */
  agentName?: string | null;
  /** True when the search spans the whole task, enabling cross-session rows. */
  taskScoped?: boolean;
  /** The session whose chat is currently shown; other session hits activate it. */
  currentSessionId?: string | null;
  hasMore?: boolean;
  onLoadMore?: () => void;
  /** Fill the parent instead of the fixed desktop popover size (phone surface). */
  fullHeight?: boolean;
};

function formatTime(iso: string): string {
  const d = new Date(iso);
  // Invalid Date doesn't throw — it silently produces NaN. Fall back to the
  // raw ISO string instead of rendering "Invalid Date".
  if (Number.isNaN(d.getTime())) return iso;
  try {
    return d.toLocaleString(undefined, {
      month: "short",
      day: "numeric",
      hour: "2-digit",
      minute: "2-digit",
    });
  } catch {
    return iso;
  }
}

/** Resolve a turn-start step id to its display name from the workflow store.
 *  The id is the authority; an unresolvable (removed) step stays unlabeled. */
function useStepName(stepId: string | null | undefined): string | null {
  return useAppStore((state) => {
    if (!stepId) return null;
    const active = state.kanban.steps.find((step) => step.id === stepId);
    if (active) return active.title;
    for (const snapshot of Object.values(state.kanbanMulti.snapshots)) {
      const match = snapshot?.steps.find((step) => step.id === stepId);
      if (match) return match.title;
    }
    return null;
  });
}

function HighlightedSnippet({ text, query }: { text: string; query: string }) {
  const parts = useMemo(() => {
    const q = query.trim();
    if (!q) return [{ text, match: false }];
    const escaped = q.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
    const re = new RegExp(escaped, "gi");
    const result: Array<{ text: string; match: boolean }> = [];
    let last = 0;
    let m: RegExpExecArray | null;
    while ((m = re.exec(text)) !== null) {
      if (m.index > last) result.push({ text: text.slice(last, m.index), match: false });
      result.push({ text: m[0], match: true });
      last = m.index + m[0].length;
      if (m.index === re.lastIndex) re.lastIndex++;
    }
    if (last < text.length) result.push({ text: text.slice(last), match: false });
    return result;
  }, [text, query]);

  return (
    <span>
      {parts.map((p, i) =>
        p.match ? (
          <mark
            key={i}
            className="bg-yellow-200/80 dark:bg-yellow-500/40 text-foreground rounded-sm px-0.5"
          >
            {p.text}
          </mark>
        ) : (
          <span key={i}>{p.text}</span>
        ),
      )}
    </span>
  );
}

function HitAuthor({
  authorType,
  agentLabel,
  agentName,
}: {
  authorType: string;
  agentLabel?: string | null;
  agentName?: string | null;
}) {
  const { t } = useTranslation();
  if (authorType === "user") {
    return (
      <span className="text-[0.6875rem] uppercase tracking-wide font-medium text-primary/80 truncate">
        {t("task:you")}
      </span>
    );
  }
  const label = agentLabel?.trim() || t("common:agent");
  return (
    <span className="text-[0.6875rem] uppercase tracking-wide font-medium text-muted-foreground inline-flex items-center gap-1.5 min-w-0">
      {agentName && <AgentLogo agentName={agentName} size={12} className="shrink-0" />}
      <span className="truncate">{label}</span>
    </span>
  );
}

/** Session + turn-start step provenance line for a task-scope hit. */
function HitMeta({
  hit,
  currentSessionId,
}: {
  hit: MessageSearchHit;
  currentSessionId?: string | null;
}) {
  const { t } = useTranslation();
  const stepName = useStepName(hit.workflow_step_id);
  const crossSession = Boolean(
    currentSessionId && hit.session_id && hit.session_id !== currentSessionId,
  );
  const sessionLabel = hit.session_name || hit.session_id;
  return (
    <div className="mb-0.5 flex items-center gap-1.5 truncate text-[0.6875rem] text-muted-foreground">
      {sessionLabel && (
        <span className="truncate">{t("task:sourceSession", { name: sessionLabel })}</span>
      )}
      {stepName && <span className="truncate">{t("task:sourceStep", { name: stepName })}</span>}
      {crossSession && (
        <span
          className="shrink-0 rounded-sm bg-primary/10 px-1 font-medium text-primary"
          data-testid={`search-hit-cross-session-${hit.id}`}
        >
          {t("task:searchOpenSession")}
        </span>
      )}
    </div>
  );
}

export function SessionSearchHits({
  hits,
  query,
  activeHitId,
  onSelect,
  isSearching,
  agentLabel,
  agentName,
  taskScoped = false,
  currentSessionId,
  hasMore = false,
  onLoadMore,
  fullHeight = false,
}: SessionSearchHitsProps) {
  const { t } = useTranslation();
  if (!query.trim()) return null;
  return (
    <div
      data-testid="session-search-hits"
      className={cn(
        "overflow-auto rounded-md border border-border bg-background shadow-lg text-xs",
        fullHeight
          ? "h-full w-full rounded-none border-0 shadow-none"
          : "max-h-80 w-[28rem] max-w-[calc(100vw-1rem)]",
      )}
    >
      {isSearching && hits.length === 0 && (
        <div className="p-3 text-muted-foreground">{t("task:searching")}</div>
      )}
      {!isSearching && hits.length === 0 && (
        <div className="p-3 text-muted-foreground">{t("task:noMatches")}</div>
      )}
      {hits.map((hit) => (
        <button
          key={hit.id}
          type="button"
          onClick={() => onSelect(hit.id)}
          className={cn(
            "w-full text-left px-3 py-2 border-b border-border last:border-0 cursor-pointer hover:bg-muted/50 max-md:min-h-11 [@media(pointer:coarse)]:min-h-11",
            activeHitId === hit.id && "bg-muted",
          )}
        >
          {taskScoped && <HitMeta hit={hit} currentSessionId={currentSessionId} />}
          <div className="flex items-center justify-between gap-2 mb-0.5">
            <HitAuthor authorType={hit.author_type} agentLabel={agentLabel} agentName={agentName} />
            <span className="text-[0.6875rem] text-muted-foreground/70 shrink-0">
              {formatTime(hit.created_at)}
            </span>
          </div>
          <div className="text-foreground/90 line-clamp-3 whitespace-pre-wrap break-words">
            <HighlightedSnippet text={hit.snippet} query={query} />
          </div>
        </button>
      ))}
      {taskScoped && hasMore && (
        <button
          type="button"
          onClick={onLoadMore}
          className="w-full cursor-pointer px-3 py-2 text-center text-primary hover:bg-muted/50"
          data-testid="search-load-more"
        >
          {t("task:searchLoadMore")}
        </button>
      )}
    </div>
  );
}
