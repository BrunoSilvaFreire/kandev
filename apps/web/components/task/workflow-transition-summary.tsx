"use client";

import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { IconArrowRight, IconHandMove } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { cn } from "@/lib/utils";
import { deriveWorkflowTransitionEdges } from "@/lib/task/workflow-transition-edges";
import { StepVisitSessionList } from "./step-visit-session-list";
import type { WorkflowStepperStep } from "./workflow-step-disclosure";
import type { TransitionSummaryState } from "@/hooks/domains/task/use-task-activity";

type WorkflowTransitionSummaryProps = {
  steps: WorkflowStepperStep[];
  currentStepId: string | null;
  summary: Pick<TransitionSummaryState, "counts" | "visits">;
  /** Activate the persisted destination session of a step visit. */
  onOpenSession?: (sessionId: string) => void;
  /** Open the Task History surface filtered to one destination step. */
  onOpenHistory?: (stepId: string) => void;
};

/**
 * Semantic, accessible counterpart of the expanded header graph. It keeps the
 * linear step order in the header and renders each currently-possible directed
 * edge once with its task-local committed count, including 0. Manual
 * destinations are dashed and labelled. No graph dependency, no horizontal
 * overflow (Header graph gate fallback).
 */
export function WorkflowTransitionSummary({
  steps,
  currentStepId,
  summary,
  onOpenSession,
  onOpenHistory,
}: WorkflowTransitionSummaryProps) {
  const { t } = useTranslation();
  const nameById = useMemo(() => new Map(steps.map((step) => [step.id, step.name])), [steps]);
  const edges = useMemo(
    () => deriveWorkflowTransitionEdges(steps, currentStepId),
    [steps, currentStepId],
  );
  const nameFor = (stepId: string) => nameById.get(stepId) ?? stepId;

  return (
    <div className="space-y-3 text-xs" data-testid="workflow-transition-summary">
      <section aria-label={t("task:workflowTransitionsTitle")}>
        <h4 className="mb-1 font-medium text-muted-foreground">
          {t("task:workflowTransitionsTitle")}
        </h4>
        {edges.length === 0 ? (
          <p className="text-muted-foreground">{t("task:workflowTransitionsEmpty")}</p>
        ) : (
          <ul className="space-y-1" data-testid="workflow-transition-edges">
            {edges.map((edge) => {
              const count = summary.counts[`${edge.fromStepId}->${edge.toStepId}`] ?? 0;
              return (
                <li
                  key={`${edge.fromStepId}->${edge.toStepId}`}
                  data-testid="workflow-transition-edge"
                  data-manual={edge.manual ? "true" : "false"}
                  aria-label={t("task:workflowTransitionLabel", {
                    from: nameFor(edge.fromStepId),
                    to: nameFor(edge.toStepId),
                    total: count,
                  })}
                  className="flex items-center gap-2"
                >
                  <span className="truncate">{nameFor(edge.fromStepId)}</span>
                  <span
                    aria-hidden="true"
                    className={cn(
                      "h-px w-5 shrink-0 border-t border-border",
                      edge.manual && "border-dashed",
                    )}
                  />
                  <IconArrowRight
                    aria-hidden="true"
                    className="h-3 w-3 shrink-0 text-muted-foreground"
                  />
                  <span className="truncate">{nameFor(edge.toStepId)}</span>
                  {edge.manual && (
                    <IconHandMove
                      aria-hidden="true"
                      className="h-3 w-3 shrink-0 text-muted-foreground"
                    />
                  )}
                  <span
                    className="ml-auto shrink-0 tabular-nums text-muted-foreground"
                    data-testid="workflow-transition-count"
                  >
                    {count}
                  </span>
                </li>
              );
            })}
          </ul>
        )}
      </section>

      <StepVisitsSection
        steps={steps}
        visits={summary.visits}
        onOpenSession={onOpenSession}
        onOpenHistory={onOpenHistory}
      />
    </div>
  );
}

/** Per-step visit counts with the History filter and one row per destination session. */
function StepVisitsSection({
  steps,
  visits,
  onOpenSession,
  onOpenHistory,
}: {
  steps: WorkflowStepperStep[];
  visits: TransitionSummaryState["visits"];
  onOpenSession?: (sessionId: string) => void;
  onOpenHistory?: (stepId: string) => void;
}) {
  const { t } = useTranslation();
  const visited = steps.filter((step) => (visits[step.id]?.count ?? 0) > 0);
  return (
    <section aria-label={t("task:workflowVisitsTitle")}>
      <h4 className="mb-1 font-medium text-muted-foreground">{t("task:workflowVisitsTitle")}</h4>
      <ul className="space-y-2" data-testid="workflow-step-visits">
        {visited.map((step) => {
          const visit = visits[step.id];
          return (
            <li key={step.id} data-testid={`workflow-step-visit-${step.id}`} className="space-y-1">
              <div className="flex items-center gap-2">
                <span className="truncate">{step.name}</span>
                <span className="shrink-0 tabular-nums text-muted-foreground">
                  {t("task:workflowTransitionCount", { count: visit?.count ?? 0 })}
                </span>
                {onOpenHistory && (
                  <Button
                    variant="ghost"
                    size="sm"
                    className="ml-auto h-6 cursor-pointer px-2 text-xs [@media(pointer:coarse)]:h-11 [@media(pointer:coarse)]:min-w-11"
                    onClick={() => onOpenHistory(step.id)}
                    data-testid={`workflow-step-visit-history-${step.id}`}
                  >
                    {t("task:panelTaskHistory")}
                  </Button>
                )}
              </div>
              <StepVisitSessionList
                sessions={visit?.sessions ?? []}
                onOpenSession={onOpenSession}
                testIdPrefix={`workflow-step-visit-session-${step.id}`}
                className="pl-3"
              />
            </li>
          );
        })}
        {visited.length === 0 && (
          <li className="text-muted-foreground">{t("task:workflowNoVisits")}</li>
        )}
      </ul>
    </section>
  );
}
