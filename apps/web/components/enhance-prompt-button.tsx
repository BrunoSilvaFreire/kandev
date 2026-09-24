"use client";

import { IconSparkles } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { GridSpinner } from "@/components/grid-spinner";
import { useTooltipMountGate } from "@/hooks/use-tooltip-mount-gate";
import { useTranslation } from "react-i18next";
import type { ExecutePromptProgress } from "@/lib/api/domains/utility-api";

type EnhancePromptButtonProps = {
  onClick: () => void;
  isLoading: boolean;
  isConfigured?: boolean;
  phase?: ExecutePromptProgress | null;
};

const PHASE_KEYS: Record<ExecutePromptProgress["phase"], string> = {
  starting: "common:enhancePromptPhaseStarting",
  analyzing: "common:enhancePromptPhaseAnalyzing",
  generating: "common:enhancePromptPhaseGenerating",
  tool: "common:enhancePromptPhaseTool",
  completed: "common:enhancePromptPhaseCompleted",
  failed: "common:enhancePromptPhaseFailed",
};

export function EnhancePromptButton({
  onClick,
  isLoading,
  isConfigured = true,
  phase,
}: EnhancePromptButtonProps) {
  const { t } = useTranslation();
  const { tooltipOpenState, handleTooltipOpenChange } = useTooltipMountGate();
  const isDisabled = !isConfigured || isLoading;
  const tooltipText = isConfigured
    ? t("common:enhancePromptWithAi")
    : t("common:configureAUtilityAgentToEnhance");
  const phaseLabel = phase ? t(PHASE_KEYS[phase.phase]) : "";

  return (
    <Tooltip open={tooltipOpenState} onOpenChange={handleTooltipOpenChange}>
      <TooltipTrigger asChild>
        {/* Wrap in span so tooltip works even when button is disabled */}
        <span
          className="inline-flex items-center gap-1"
          tabIndex={isDisabled ? 0 : -1}
          aria-label={isDisabled ? tooltipText : undefined}
        >
          <span aria-hidden={isDisabled ? "true" : undefined} className="inline-flex">
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="h-7 w-7 cursor-pointer text-slate-400 hover:bg-muted/40 [@media(pointer:coarse)]:min-h-11 [@media(pointer:coarse)]:min-w-11"
              onClick={isConfigured ? onClick : undefined}
              disabled={isDisabled}
              aria-label={t("common:enhancePromptWithAi")}
              aria-busy={isLoading}
              data-testid="enhance-prompt-button"
            >
              {isLoading ? (
                <GridSpinner className="h-4 w-4" />
              ) : (
                <IconSparkles className="h-4 w-4" />
              )}
            </Button>
          </span>
          <span className="sr-only" aria-live="polite">
            {phaseLabel}
          </span>
          {phaseLabel ? (
            <span className="hidden text-xs text-slate-400 sm:inline">{phaseLabel}</span>
          ) : null}
        </span>
      </TooltipTrigger>
      <TooltipContent>{tooltipText}</TooltipContent>
    </Tooltip>
  );
}
