"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconChevronRight, IconX } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { CardContent } from "@kandev/ui/card";
import { SettingsCard } from "@/components/settings/settings-card";
import { SettingsCardHeader } from "@/components/settings/settings-card-header";
import {
  SettingsFieldDescription,
  SettingsFieldLabel,
} from "@/components/settings/settings-typography";
import { TagTokenInput } from "@/components/settings/tag-token-input";
import { hasOnEnterAction, HelpTip } from "@/components/settings/workflow-pipeline-editor-helpers";
import { MobilePickerSheet } from "@/components/task/mobile/mobile-picker-sheet";
import { useAppStore } from "@/components/state-provider";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useAgentProfileUtilization } from "@/hooks/domains/settings/use-agent-profile-utilization";
import type { WorkflowStep } from "@/lib/types/http";

type WorkflowStepAllowedTagsProps = {
  step: WorkflowStep;
  onUpdate: (updates: Partial<WorkflowStep>) => void;
  readOnly: boolean;
};

type CandidateProfile = { id: string; label: string; tags: string[] };

function matchesAllowedTags(profileTags: string[], allowedTags: string[]): boolean {
  const allowed = new Set(allowedTags);
  return profileTags.some((tag) => allowed.has(tag));
}

function useCandidateProfiles(allowedTags: string[]): CandidateProfile[] {
  const agents = useAppStore((state) => state.settingsAgents.items);
  return useMemo(() => {
    if (allowedTags.length === 0) return [];
    const candidates: CandidateProfile[] = [];
    for (const agent of agents) {
      for (const profile of agent.profiles ?? []) {
        if (
          profile.enabled === false ||
          profile.kind === "dynamic" ||
          profile.workspaceId ||
          profile.role
        ) {
          continue;
        }
        const tags = profile.tags ?? [];
        if (!matchesAllowedTags(tags, allowedTags)) continue;
        candidates.push({
          id: profile.id,
          label: profile.name || profile.agentDisplayName,
          tags,
        });
      }
    }
    return candidates.sort((left, right) => left.id.localeCompare(right.id));
  }, [agents, allowedTags]);
}

export function WorkflowStepAllowedTags({ step, onUpdate, readOnly }: WorkflowStepAllowedTagsProps) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const allowedTags = step.allowed_tags ?? [];
  const hasSessionTarget = Boolean(step.session_target);
  const hasConfigureSession = hasOnEnterAction(step, "configure_session");
  const conflicted = hasSessionTarget || hasConfigureSession;
  const editable = !readOnly && !conflicted;
  const candidates = useCandidateProfiles(allowedTags);
  const showPreview = !conflicted && allowedTags.length > 0;
  // Read-only workflows still show the candidate surface: editability and
  // preview visibility are separate concerns (AC-001.13).
  const { items, loading, error } = useAgentProfileUtilization(
    candidates.map((candidate) => candidate.id),
    showPreview && candidates.length > 0,
  );

  let disabledReason: string | null = null;
  if (hasSessionTarget) {
    disabledReason = t("workflows:allowedTagsSessionTargetConflict");
  } else if (hasConfigureSession) {
    disabledReason = t("workflows:allowedTagsConfigureSessionConflict");
  }

  const previewProps = { stepId: step.id, candidates, items, loading, error };

  return (
    <SettingsCard isDirty={false} discoveryTargetId={`${step.id}-allowed-tags`}>
      <SettingsCardHeader
        title={t("workflows:allowedTagsTitle")}
        description={t("workflows:allowedTagsDescription")}
        actions={
          <HelpTip
            testId={`${step.id}-allowed-tags-help`}
            text={disabledReason ?? t("workflows:allowedTagsHelp")}
          />
        }
      />
      <CardContent className="space-y-3">
        <SettingsFieldLabel>{t("workflows:allowedTagsLabel")}</SettingsFieldLabel>
        <TagTokenInput
          tags={allowedTags}
          onChange={(tags) => editable && onUpdate({ allowed_tags: tags })}
          placeholder={t("workflows:allowedTagsPlaceholder")}
          removeLabel={(tag) => t("workflows:allowedTagsRemove", { tag })}
          disabled={!editable}
          listTestId={`${step.id}-allowed-tags-list`}
          inputTestId={`${step.id}-allowed-tags-input`}
        />
        <SettingsFieldDescription>
          {disabledReason ?? t("workflows:allowedTagsHelp")}
        </SettingsFieldDescription>
        {showPreview && allowedTags.length > 0 && (
          <SettingsFieldDescription data-testid={`${step.id}-allowed-tags-next-entry`}>
            {t("workflows:allowedTagsAppliesNextEntry")}
          </SettingsFieldDescription>
        )}
        {showPreview &&
          (isMobile ? (
            <MobileCandidatePreview {...previewProps} />
          ) : (
            <CandidatePreview {...previewProps} showTitle />
          ))}
      </CardContent>
    </SettingsCard>
  );
}

type CandidatePreviewProps = {
  stepId: string;
  candidates: CandidateProfile[];
  items: Record<string, { state: string; remaining_pct?: number }>;
  loading: boolean;
  error: unknown;
};

function candidateLabel(
  item: { state?: string; remaining_pct?: number } | undefined,
  loading: boolean,
  t: (key: string, options?: Record<string, unknown>) => string,
): string {
  if (item?.state === "known" && typeof item.remaining_pct === "number") {
    return t("workflows:candidateRemaining", { pct: Math.round(item.remaining_pct) });
  }
  if (loading) return t("workflows:candidateLoading");
  if (item?.state === "unavailable") return t("workflows:candidateUnavailable");
  return t("workflows:candidateUnknown");
}

function CandidateList({ candidates, items, loading }: CandidatePreviewProps) {
  const { t } = useTranslation();
  if (candidates.length === 0) {
    return <SettingsFieldDescription>{t("workflows:candidateEmpty")}</SettingsFieldDescription>;
  }
  return (
    <ul className="space-y-1">
      {candidates.map((candidate) => {
        const item = items[candidate.id];
        const label = candidateLabel(item, loading, t);
        return (
          <li
            key={candidate.id}
            className="flex min-h-11 items-center justify-between gap-2 text-xs text-muted-foreground md:min-h-0"
          >
            <span className="min-w-0 truncate">{candidate.label}</span>
            <Badge
              variant="outline"
              className="shrink-0"
              aria-label={t("workflows:candidateStateLabel", { name: candidate.label, state: label })}
            >
              {label}
            </Badge>
          </li>
        );
      })}
    </ul>
  );
}

function CandidatePreview({
  showTitle,
  ...props
}: CandidatePreviewProps & { showTitle: boolean }) {
  const { t } = useTranslation();
  return (
    <div className="space-y-2" data-testid={`${props.stepId}-candidate-preview`}>
      {showTitle ? (
        <SettingsFieldLabel>{t("workflows:candidatePreviewTitle")}</SettingsFieldLabel>
      ) : null}
      {props.error ? (
        <SettingsFieldDescription role="status">
          {t("workflows:candidateError")}
        </SettingsFieldDescription>
      ) : null}
      <CandidateList {...props} />
    </div>
  );
}

function MobileCandidatePreview(props: CandidatePreviewProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const wasOpenRef = useRef(false);

  useEffect(() => {
    if (wasOpenRef.current && !open) {
      requestAnimationFrame(() => triggerRef.current?.focus({ preventScroll: true }));
    }
    wasOpenRef.current = open;
  }, [open]);

  return (
    <>
      <Button
        ref={triggerRef}
        type="button"
        variant="outline"
        className="h-11 w-full cursor-pointer justify-between"
        data-testid={`${props.stepId}-candidate-preview-trigger`}
        aria-haspopup="dialog"
        aria-expanded={open}
        onClick={() => setOpen(true)}
      >
        <span className="min-w-0 truncate">{t("workflows:candidatePreviewTitle")}</span>
        <span className="flex shrink-0 items-center gap-2">
          <Badge variant="outline">{props.candidates.length}</Badge>
          <IconChevronRight className="h-4 w-4" aria-hidden="true" />
        </span>
      </Button>
      {open && (
        <MobilePickerSheet
          open
          onOpenChange={(next) => !next && setOpen(false)}
          title={t("workflows:candidatePreviewTitle")}
          contentTestId={`${props.stepId}-candidate-preview-sheet-content`}
          onCloseAutoFocus={(event) => {
            event.preventDefault();
            triggerRef.current?.focus({ preventScroll: true });
          }}
          headerAction={
            <Button
              type="button"
              variant="ghost"
              className="h-11 min-w-11 cursor-pointer"
              aria-label={t("common:close")}
              data-testid={`${props.stepId}-candidate-preview-close`}
              onClick={() => setOpen(false)}
            >
              <IconX className="h-4 w-4" aria-hidden="true" />
            </Button>
          }
        >
          <div className="px-2 pt-2">
            {props.error ? (
              <SettingsFieldDescription role="status">
                {t("workflows:candidateError")}
              </SettingsFieldDescription>
            ) : null}
            <CandidateList {...props} />
          </div>
        </MobilePickerSheet>
      )}
    </>
  );
}
