"use client";

import { Badge } from "@kandev/ui/badge";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { cn } from "@/lib/utils";
import type { UsageChipTone } from "./usage-format";

const TONE_STYLE: Record<UsageChipTone, string> = {
  neutral: "border-muted-foreground/40 text-muted-foreground",
  healthy: "border-emerald-500/40 text-emerald-600 dark:text-emerald-400",
  nearing: "border-amber-500/40 text-amber-600 dark:text-amber-400",
  exhausted: "border-red-500/40 text-red-600 dark:text-red-400",
};

/**
 * An outlined badge that reveals an explanatory tooltip on hover or keyboard
 * focus. tabIndex keeps the chip reachable for keyboard-only users.
 */
export function UsageInfoChip({
  label,
  tooltip,
  tone = "neutral",
  testId,
}: {
  label: string;
  tooltip: string;
  tone?: UsageChipTone;
  testId?: string;
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Badge
          variant="outline"
          tabIndex={0}
          data-testid={testId}
          className={cn("cursor-help text-xs", TONE_STYLE[tone])}
        >
          {label}
        </Badge>
      </TooltipTrigger>
      <TooltipContent>{tooltip}</TooltipContent>
    </Tooltip>
  );
}
