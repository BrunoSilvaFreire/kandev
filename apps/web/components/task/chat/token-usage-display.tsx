"use client";

import { memo, useCallback, useEffect, useId, useRef, useState, type RefObject } from "react";
import { IconChartBar, IconInfoCircle } from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@kandev/ui/tooltip";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import { cn } from "@/lib/utils";
import type { ContextWindowEntry } from "@/lib/state/store";
import { useSessionContextWindow } from "@/hooks/domains/session/use-session-context-window";
import { useSessionUsageInspector } from "@/hooks/domains/session/use-session-usage-inspector";
import { cacheStatusLabelKey, formatNumber } from "@/lib/usage/format";
import type { CacheStatus, UsageFlags } from "@/lib/usage/efficiency";
import { useClarificationEscapeGuard } from "@/hooks/use-clarification-escape-guard";
import { TokenUsageInspector } from "./token-usage-inspector";

type TokenUsageDisplayProps = {
  sessionId: string | null;
  /** Enables the usage/cost inspector sections (task-scoped routes). */
  taskId?: string | null;
  className?: string;
};

const STATUS_DOT_CLASS: Record<CacheStatus, string> = {
  warm: "bg-green-500",
  likely_expired: "bg-amber-500",
  unknown: "bg-muted-foreground",
};

function hasAnyUsageFlag(flags: UsageFlags | null): boolean {
  return !!flags && (flags.lowHitRatio || flags.highCost || flags.cacheLikelyExpired);
}

function buildTriggerAriaLabel(
  contextWindow: ContextWindowEntry | undefined,
  contextReliable: boolean,
  statusText: string | null,
  t: TFunction,
): string {
  let contextLabel: string | null = null;
  if (contextReliable && contextWindow) {
    contextLabel =
      contextWindow.used === 0
        ? t("task:contextWindowNotMeasured")
        : t("task:contextWindowUsed", {
            percent: ((contextWindow.used / contextWindow.size) * 100).toFixed(0),
          });
  }
  return [contextLabel, statusText].filter(Boolean).join(". ");
}

/**
 * A context-window report is only trustworthy when we have a positive window
 * size and usage that does not exceed it. `used > size` is impossible for a
 * real window, so it means the agent (via the ACP bridge) reported a stale or
 * wrong `size` (for example, usage and window metadata from different turns).
 * In that case we hide the indicator instead of showing a confusing >100%.
 * `used === size` (exactly full) is valid and still renders.
 */
export function isContextWindowReliable(size: number, used: number): boolean {
  return size > 0 && used <= size;
}

function getCircleColor(efficiency: number): string {
  if (efficiency >= 90) return "text-yellow-500";
  if (efficiency >= 75) return "text-yellow-300";
  if (efficiency >= 50) return "text-blue-500";
  return "text-blue-300";
}

function usePinnableTooltip() {
  const [open, setOpen] = useState(false);
  const pinnedRef = useRef(false);
  const triggerRef = useRef<HTMLButtonElement>(null);

  const escapeGuard = useCallback(
    (event: KeyboardEvent) => event.key === "Escape" && pinnedRef.current,
    [],
  );
  // Radix dialogs inspect Escape during document capture, before the
  // document-bubble listener below can close the pinned tooltip.
  useClarificationEscapeGuard(escapeGuard);

  useEffect(() => {
    const closePinnedTooltip = () => {
      pinnedRef.current = false;
      setOpen(false);
    };
    const closeOnOutsidePointer = (event: PointerEvent) => {
      if (!pinnedRef.current || !(event.target instanceof Node)) return;
      if (
        triggerRef.current?.contains(event.target) ||
        (event.target instanceof Element && event.target.closest('[data-slot="tooltip-content"]'))
      ) {
        return;
      }
      closePinnedTooltip();
    };
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key !== "Escape" || !pinnedRef.current) return;
      // Claim the key here: once the pinned tooltip is closed, nothing
      // further up the tree (e.g. a clarification panel's own
      // Escape-collapses handler) should also react to the same keypress.
      event.preventDefault();
      event.stopPropagation();
      closePinnedTooltip();
    };

    document.addEventListener("pointerdown", closeOnOutsidePointer);
    document.addEventListener("keydown", closeOnEscape);
    return () => {
      document.removeEventListener("pointerdown", closeOnOutsidePointer);
      document.removeEventListener("keydown", closeOnEscape);
    };
  }, []);

  return {
    open,
    triggerRef,
    onOpenChange: (nextOpen: boolean) => {
      if (!nextOpen && pinnedRef.current) return;
      setOpen(nextOpen);
    },
    onTriggerClick: () => {
      pinnedRef.current = !pinnedRef.current;
      setOpen(pinnedRef.current);
    },
  };
}

function ContextWindowRing({ usagePercent }: { usagePercent: number }) {
  const radius = 10;
  const strokeWidth = 2.5;
  const circumference = 2 * Math.PI * radius;
  const strokeDashoffset = circumference * (1 - usagePercent / 100);

  return (
    <svg viewBox="0 0 24 24" className="size-5 -rotate-90" aria-hidden="true">
      <circle
        cx="12"
        cy="12"
        r={radius}
        fill="none"
        stroke="currentColor"
        strokeWidth={strokeWidth}
        className="text-muted"
      />
      <circle
        cx="12"
        cy="12"
        r={radius}
        fill="none"
        stroke="currentColor"
        strokeWidth={strokeWidth}
        strokeLinecap="round"
        strokeDasharray={circumference}
        strokeDashoffset={strokeDashoffset}
        className={cn(
          getCircleColor(usagePercent),
          "transition-[stroke-dashoffset] duration-300 ease-out",
        )}
      />
    </svg>
  );
}

function ContextWindowSource({ source }: { source: "acp" | "api" | undefined }) {
  const { t } = useTranslation();
  const helpId = useId();
  const [helpOpen, setHelpOpen] = useState(false);
  const [touchMode, setTouchMode] = useState(false);
  const pointerDownRef = useRef(false);

  if (!source) return null;
  const description =
    source === "acp"
      ? "ACP is the active session's effective window, reported by the agent."
      : "API is the model's advertised maximum from the catalogue and is used when ACP omits the window.";

  return (
    <div className="group relative flex shrink-0 items-center gap-1 text-[10px] text-muted-foreground">
      <span>{t("task:source")}</span>
      <span className="font-medium text-foreground">{source.toUpperCase()}</span>
      <button
        type="button"
        aria-label={t("task:aboutContextWindowSource")}
        aria-describedby={helpId}
        aria-expanded={helpOpen}
        className="inline-flex size-6 cursor-help items-center justify-center text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring sm:size-4"
        onPointerDown={(event) => {
          pointerDownRef.current = true;
          if (event.pointerType !== "touch") setTouchMode(false);
        }}
        onTouchStart={() => {
          setTouchMode(true);
        }}
        onFocus={() => {
          if (!pointerDownRef.current) setHelpOpen(true);
        }}
        onBlur={() => {
          pointerDownRef.current = false;
          setHelpOpen(false);
        }}
        onClick={() => {
          pointerDownRef.current = false;
          setHelpOpen((open) => !open);
        }}
      >
        <IconInfoCircle className="h-3 w-3" />
      </button>
      <span
        id={helpId}
        role="tooltip"
        className={cn(
          "pointer-events-none absolute right-0 bottom-[calc(100%+0.375rem)] z-10 w-60 rounded-md border border-border bg-popover px-3 py-1.5 text-xs text-popover-foreground opacity-0 shadow-sm transition-opacity",
          !touchMode && "group-hover:opacity-100",
          helpOpen && "opacity-100",
        )}
      >
        {description}
      </span>
    </div>
  );
}

function ContextCompactionCount({ count }: { count: number }) {
  const { t } = useTranslation();
  const helpId = useId();
  const [helpOpen, setHelpOpen] = useState(false);
  const [touchMode, setTouchMode] = useState(false);
  const pointerDownRef = useRef(false);

  return (
    <div
      className="flex min-h-6 items-center justify-between gap-3"
      data-testid="context-window-compactions-row"
    >
      <span className="text-[11px] text-muted-foreground">{t("common:contextCompactions")}</span>
      <div className="group relative flex shrink-0 items-center gap-1 text-[11px] text-muted-foreground">
        <span className="font-medium tabular-nums text-foreground">{count}</span>
        <button
          type="button"
          aria-label={t("common:aboutContextCompactionCount")}
          aria-describedby={helpId}
          aria-expanded={helpOpen}
          className="inline-flex size-6 cursor-help items-center justify-center text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring sm:size-4"
          onPointerDown={(event) => {
            pointerDownRef.current = true;
            if (event.pointerType !== "touch") setTouchMode(false);
          }}
          onTouchStart={() => {
            setTouchMode(true);
          }}
          onFocus={() => {
            if (!pointerDownRef.current) setHelpOpen(true);
          }}
          onBlur={() => {
            pointerDownRef.current = false;
            setHelpOpen(false);
          }}
          onClick={() => {
            pointerDownRef.current = false;
            setHelpOpen((open) => !open);
          }}
        >
          <IconInfoCircle className="h-3 w-3" />
        </button>
        <span
          id={helpId}
          role="tooltip"
          className={cn(
            "pointer-events-none absolute right-0 bottom-[calc(100%+0.375rem)] z-10 w-60 rounded-md border border-border bg-popover px-3 py-1.5 text-xs text-popover-foreground opacity-0 shadow-sm transition-opacity",
            !touchMode && "group-hover:opacity-100",
            helpOpen && "opacity-100",
          )}
        >
          {t("common:contextCompactionCountHelp")}
        </span>
      </div>
    </div>
  );
}

function ContextWindowSection({
  size,
  used,
  source,
  compactionCount,
}: {
  size: number;
  used: number;
  source: "acp" | "api" | undefined;
  compactionCount: number;
}) {
  const { t } = useTranslation();
  const usagePercent = (used / size) * 100;
  const pendingUsage = used === 0;
  const usagePercentLabel = pendingUsage
    ? t("task:contextWindowUsagePendingPercent")
    : `${usagePercent.toFixed(0)}%`;
  const usageTokenLabel = pendingUsage
    ? t("task:contextWindowUsagePendingTokens", { size: formatNumber(size) })
    : t("task:contextWindowUsedTokens", { used: formatNumber(used), size: formatNumber(size) });

  return (
    <div className="space-y-2" data-testid="context-window-usage">
      <div className="flex items-baseline justify-between gap-6">
        <span className="text-[10px] font-medium uppercase text-muted-foreground">
          {t("task:contextWindow")}
        </span>
        <span className="text-base font-semibold tabular-nums text-foreground">
          {usagePercentLabel}
        </span>
      </div>
      <div
        className={cn("h-1.5 overflow-hidden rounded-full bg-muted", getCircleColor(usagePercent))}
      >
        <div
          className="h-full rounded-full bg-current transition-all duration-300 ease-out"
          style={{ width: `${usagePercent}%` }}
        />
      </div>
      <div
        className="flex min-h-6 items-center justify-between gap-3"
        data-testid="context-window-token-row"
      >
        <span className="text-[11px] tabular-nums text-muted-foreground">{usageTokenLabel}</span>
        <ContextWindowSource source={source} />
      </div>
      {pendingUsage && (
        <p className="text-[11px] text-muted-foreground" data-testid="context-window-usage-pending">
          {t("task:contextWindowUsagePendingExplain")}
        </p>
      )}
      <ContextCompactionCount count={compactionCount} />
    </div>
  );
}

type TokenUsageTriggerProps = {
  triggerRef: RefObject<HTMLButtonElement | null>;
  open: boolean;
  onTriggerClick: () => void;
  ariaLabel: string;
  flagged: boolean;
  showDot: boolean;
  contextReliable: boolean;
  contextWindow: ContextWindowEntry | undefined;
  status: CacheStatus;
  className?: string;
};

function TokenUsageTrigger({
  triggerRef,
  open,
  onTriggerClick,
  ariaLabel,
  flagged,
  showDot,
  contextReliable,
  contextWindow,
  status,
  className,
}: TokenUsageTriggerProps) {
  return (
    <TooltipTrigger asChild>
      <button
        ref={triggerRef}
        type="button"
        aria-label={ariaLabel}
        aria-expanded={open}
        onClick={onTriggerClick}
        className={cn(
          "relative flex size-7 cursor-help items-center justify-center rounded-sm focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring sm:size-5",
          flagged && "bg-amber-500/10",
          className,
        )}
      >
        {contextReliable && contextWindow ? (
          <ContextWindowRing usagePercent={(contextWindow.used / contextWindow.size) * 100} />
        ) : (
          <IconChartBar className="size-4 text-muted-foreground" />
        )}
        {showDot ? (
          <span
            data-testid="usage-cache-status-dot"
            data-cache-status={status}
            aria-hidden="true"
            className={cn(
              "absolute -right-0.5 -top-0.5 size-1.5 rounded-full",
              STATUS_DOT_CLASS[status],
            )}
          />
        ) : null}
      </button>
    </TooltipTrigger>
  );
}

export const TokenUsageDisplay = memo(function TokenUsageDisplay({
  sessionId,
  taskId,
  className,
}: TokenUsageDisplayProps) {
  const { t } = useTranslation();
  const tooltip = usePinnableTooltip();
  const contextWindow = useSessionContextWindow(sessionId);
  const inspector = useSessionUsageInspector(taskId ?? null, sessionId, {
    enabled: Boolean(taskId && sessionId),
  });

  const contextReliable = contextWindow
    ? isContextWindowReliable(contextWindow.size, contextWindow.used)
    : false;
  const hasUsage = (inspector.session?.event_count ?? 0) > 0;
  // The indicator must still render when only usage data exists.
  if (!contextReliable && !hasUsage) return null;

  // The inspector (dot, status text, flag tint, usage sections) is task-chat
  // only; quick chat keeps the context-only indicator.
  const showInspector = Boolean(taskId && sessionId);
  const statusText = showInspector
    ? t("task:usageInspectorTriggerLabel", {
        status: t(cacheStatusLabelKey(inspector.status)),
      })
    : null;
  const ariaLabel = buildTriggerAriaLabel(contextWindow, contextReliable, statusText, t);
  const flagged = showInspector && hasAnyUsageFlag(inspector.flags);

  return (
    // The UI wrapper defaults this to true; the source control must remain reachable inside.
    <TooltipProvider disableHoverableContent={false}>
      <Tooltip open={tooltip.open} onOpenChange={tooltip.onOpenChange}>
        <TokenUsageTrigger
          triggerRef={tooltip.triggerRef}
          open={tooltip.open}
          onTriggerClick={tooltip.onTriggerClick}
          ariaLabel={ariaLabel}
          flagged={flagged}
          showDot={showInspector}
          contextReliable={contextReliable}
          contextWindow={contextWindow}
          status={inspector.status}
          className={className}
        />
        <TooltipContent side="top" className="pointer-events-auto max-h-[70vh] overflow-y-auto">
          <div className="min-w-64 text-xs">
            {contextReliable && contextWindow ? (
              <ContextWindowSection
                size={contextWindow.size}
                used={contextWindow.used}
                source={contextWindow.source}
                compactionCount={contextWindow.compactionCount}
              />
            ) : null}
            {taskId ? <TokenUsageInspector inspector={inspector} /> : null}
          </div>
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
});
