"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { IconChevronDown, IconFileText } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import { Drawer, DrawerContent, DrawerHeader, DrawerTitle } from "@kandev/ui/drawer";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { transitionReasonKey, type TransitionFailure } from "@/lib/tasks/transition-failure";

function ReasonList({
  failure,
  onOpenPlan,
}: {
  failure: TransitionFailure;
  onOpenPlan?: () => void;
}) {
  const { t } = useTranslation();
  const offersPlan = failure.reasons.some((reason) => reason.cta?.kind === "open_plan");
  return (
    <div className="space-y-2 text-xs" data-testid="transition-failure-reasons">
      <Badge variant="outline" data-testid="transition-failure-actionability">
        {failure.actionability === "system"
          ? t("task:transitionFailureSystemFault")
          : t("task:transitionFailureUserFault")}
      </Badge>
      <ul className="list-disc space-y-1.5 pl-4">
        {failure.reasons.map((reason, index) => (
          <li
            key={`${reason.rule}:${reason.criterion_id ?? index}`}
            data-testid="transition-failure-reason"
            data-rule={reason.rule}
          >
            {reason.criterion_id && (
              <span className="font-medium">
                {t("task:transitionFailureCriterion", { id: reason.criterion_id })}
                {reason.description ? `: ${reason.description}. ` : ". "}
              </span>
            )}
            {t(transitionReasonKey(reason.rule))}
          </li>
        ))}
      </ul>
      {failure.incident_id && (
        <p className="text-muted-foreground" data-testid="transition-failure-incident">
          {t("task:transitionFailureIncident", { id: failure.incident_id })}
        </p>
      )}
      {offersPlan && onOpenPlan && (
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="h-7 cursor-pointer gap-1 text-xs [@media(pointer:coarse)]:min-h-11"
          onClick={onOpenPlan}
          data-testid="transition-failure-open-plan"
        >
          <IconFileText className="h-3.5 w-3.5" />
          {t("task:approvalOpenPlan")}
        </Button>
      )}
    </div>
  );
}

/**
 * Expandable reasons for one transition failure. A fine pointer expands them
 * inline; a touch device opens the same content in a drawer.
 */
export function TransitionFailureDetails({
  failure,
  onOpenPlan,
}: {
  failure: TransitionFailure;
  onOpenPlan?: () => void;
}) {
  const { t } = useTranslation();
  const touch = useTouchDrawer();
  const [open, setOpen] = useState(false);
  if (failure.reasons.length === 0) return null;
  const openPlan = onOpenPlan
    ? () => {
        setOpen(false);
        onOpenPlan();
      }
    : undefined;
  const trigger = (
    <Button
      type="button"
      variant="ghost"
      size="sm"
      className="h-7 cursor-pointer gap-1 px-1 text-xs [@media(pointer:coarse)]:min-h-11"
      aria-expanded={open}
      onClick={touch ? () => setOpen(true) : undefined}
      data-testid="transition-failure-details-toggle"
    >
      <IconChevronDown className="h-3.5 w-3.5" />
      {t("task:transitionFailureDetails")}
    </Button>
  );
  if (touch) {
    return (
      <>
        {trigger}
        <Drawer open={open} onOpenChange={setOpen}>
          <DrawerContent data-testid="transition-failure-drawer">
            <DrawerHeader>
              <DrawerTitle>{t("task:transitionFailureDetails")}</DrawerTitle>
            </DrawerHeader>
            <div className="max-h-[70dvh] overflow-y-auto px-4 pb-[max(1rem,env(safe-area-inset-bottom))]">
              <ReasonList failure={failure} onOpenPlan={openPlan} />
            </div>
          </DrawerContent>
        </Drawer>
      </>
    );
  }
  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <CollapsibleTrigger asChild>{trigger}</CollapsibleTrigger>
      <CollapsibleContent className="pt-1">
        <ReasonList failure={failure} onOpenPlan={openPlan} />
      </CollapsibleContent>
    </Collapsible>
  );
}
