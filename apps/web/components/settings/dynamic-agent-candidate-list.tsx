"use client";

import { useTranslation } from "react-i18next";
import { IconArrowDown, IconArrowUp, IconTrash } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Switch } from "@kandev/ui/switch";
import { AgentProfilePicker } from "@/components/settings/agent-profile-picker";
import { DynamicPolicyEditor } from "@/components/settings/dynamic-agent-policy-editor";
import { useAgentProfileUtilization } from "@/hooks/domains/settings/use-agent-profile-utilization";
import type { AgentProfileOption } from "@/lib/state/slices/settings/types";
import type { AgentProfile } from "@/lib/types/http";
import type {
  DynamicAgentCandidate,
  DynamicErrorClass,
  DynamicErrorPolicy,
} from "@/lib/types/agent-profile";
import { settingsActionClassName, settingsControlClassName } from "./settings-control";

type DynamicAgentCandidateListProps = {
  candidates: DynamicAgentCandidate[];
  concreteProfiles: AgentProfile[];
  availableProfileOptions: AgentProfileOption[];
  enabledLabel: string;
  preferredTags: string[];
  avoidedTags: string[];
  addCandidate: (executionProfileId: string) => void;
  moveCandidate: (index: number, direction: -1 | 1) => void;
  removeCandidate: (index: number) => void;
  updateCandidate: (index: number, patch: Partial<DynamicAgentCandidate>) => void;
  updateCandidatePolicy: (
    index: number,
    errorClass: DynamicErrorClass,
    patch: Partial<DynamicErrorPolicy>,
  ) => void;
};

export type CandidatePreferenceClass = "preferred" | "avoided" | "neutral";

function candidateLabel(candidate: DynamicAgentCandidate, profiles: AgentProfile[]): string {
  return (
    profiles.find((profile) => profile.id === candidate.executionProfileId)?.name ??
    candidate.executionProfileId
  );
}

function candidateTags(candidate: DynamicAgentCandidate, profiles: AgentProfile[]): string[] {
  return profiles.find((profile) => profile.id === candidate.executionProfileId)?.tags ?? [];
}

/** Derived soft-affinity class from a candidate's concrete tags. */
export function candidatePreferenceClass(
  tags: string[],
  preferredTags: string[],
  avoidedTags: string[],
): CandidatePreferenceClass {
  const tagSet = new Set(tags);
  if (preferredTags.some((tag) => tagSet.has(tag))) return "preferred";
  if (avoidedTags.some((tag) => tagSet.has(tag))) return "avoided";
  return "neutral";
}

function preferenceLabel(
  preferenceClass: CandidatePreferenceClass,
  hasPreferences: boolean,
  t: (key: string) => string,
): string | null {
  if (preferenceClass === "preferred") return t("agents:dynamicCandidatePreferred");
  if (preferenceClass === "avoided") return t("agents:dynamicCandidateAvoided");
  return hasPreferences ? t("agents:dynamicCandidateFallback") : null;
}

function quotaLabel(
  item: { state?: string; remaining_pct?: number } | undefined,
  loading: boolean,
  t: (key: string, options?: Record<string, unknown>) => string,
): string {
  if (item?.state === "known" && typeof item.remaining_pct === "number") {
    return t("agents:dynamicCandidateRemaining", { pct: Math.round(item.remaining_pct) });
  }
  if (loading) return t("agents:dynamicCandidateQuotaLoading");
  if (item?.state === "unavailable") return t("agents:dynamicCandidateQuotaUnavailable");
  return t("agents:dynamicCandidateQuotaUnknown");
}

// eslint-disable-next-line max-lines-per-function -- keeps the candidate list row actions and policies together.
export function DynamicAgentCandidateList({
  candidates,
  concreteProfiles,
  availableProfileOptions,
  enabledLabel,
  preferredTags,
  avoidedTags,
  addCandidate,
  moveCandidate,
  removeCandidate,
  updateCandidate,
  updateCandidatePolicy,
}: DynamicAgentCandidateListProps) {
  const { t } = useTranslation();
  const hasPreferences = preferredTags.length > 0 || avoidedTags.length > 0;
  const { items, loading } = useAgentProfileUtilization(
    candidates.map((candidate) => candidate.executionProfileId),
    candidates.length > 0,
  );

  return (
    <div className="space-y-3" data-testid="dynamic-profile-candidates">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h3 className="text-sm font-medium">{t("agents:dynamicCandidates")}</h3>
          <p className="text-xs text-muted-foreground">
            {t("agents:dynamicCandidatesDescription")}
          </p>
        </div>
        <AgentProfilePicker
          profiles={availableProfileOptions}
          value=""
          onValueChange={(value) => {
            if (value) addCandidate(value);
          }}
          testId="add-dynamic-candidate"
          placeholder={t("agents:addDynamicCandidate")}
          searchPlaceholder={t("agents:searchDynamicCandidates")}
          emptyMessage={t("agents:noDynamicCandidatesFound")}
          ariaLabel={t("agents:addDynamicCandidate")}
          triggerClassName={settingsControlClassName("w-full sm:w-auto")}
        />
      </div>

      {candidates.length === 0 ? (
        <p className="rounded-md border border-dashed p-4 text-sm text-muted-foreground">
          {t("agents:noDynamicCandidates")}
        </p>
      ) : (
        <ol className="grid gap-4">
          {candidates.map((candidate, index) => {
            const tags = candidateTags(candidate, concreteProfiles);
            const preferenceClass = candidatePreferenceClass(tags, preferredTags, avoidedTags);
            const label = preferenceLabel(preferenceClass, hasPreferences, t);
            const quota = quotaLabel(items[candidate.executionProfileId], loading, t);
            return (
              <li
                key={candidate.executionProfileId}
                className="min-w-0 space-y-4 rounded-md border p-3"
              >
                <div className="flex min-w-0 flex-wrap items-center gap-3">
                  <div className="flex min-w-0 basis-full items-center gap-3 sm:basis-auto sm:flex-1">
                    <Badge variant="outline">{index + 1}</Badge>
                    <span className="min-w-0 truncate text-sm font-medium">
                      {candidateLabel(candidate, concreteProfiles)}
                    </span>
                  </div>
                  <div className="flex min-h-11 items-center gap-2 rounded-md border px-2">
                    <span className="text-xs text-muted-foreground">{enabledLabel}</span>
                    <Switch
                      checked={candidate.enabled}
                      onCheckedChange={(checked) => updateCandidate(index, { enabled: checked })}
                      aria-label={`${enabledLabel}: ${candidateLabel(candidate, concreteProfiles)}`}
                    />
                  </div>
                  <div className="flex items-center gap-1">
                    <Button
                      variant="ghost"
                      size="icon"
                      className={settingsActionClassName("shrink-0 cursor-pointer")}
                      onClick={() => moveCandidate(index, -1)}
                      disabled={index === 0}
                      aria-label={t("agents:moveDynamicCandidateUp")}
                    >
                      <IconArrowUp className="h-4 w-4" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon"
                      className={settingsActionClassName("shrink-0 cursor-pointer")}
                      onClick={() => moveCandidate(index, 1)}
                      disabled={index === candidates.length - 1}
                      aria-label={t("agents:moveDynamicCandidateDown")}
                    >
                      <IconArrowDown className="h-4 w-4" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon"
                      className={settingsActionClassName(
                        "shrink-0 cursor-pointer text-destructive",
                      )}
                      onClick={() => removeCandidate(index)}
                      aria-label={t("agents:removeDynamicCandidate")}
                    >
                      <IconTrash className="h-4 w-4" />
                    </Button>
                  </div>
                </div>
                <div
                  className="flex flex-wrap items-center gap-2"
                  data-testid={`dynamic-candidate-status-${candidate.executionProfileId}`}
                >
                  {tags.length > 0 ? (
                    <span className="flex flex-wrap gap-1">
                      {tags.map((tag) => (
                        <Badge key={tag} variant="secondary">
                          {tag}
                        </Badge>
                      ))}
                    </span>
                  ) : null}
                  <Badge variant="outline">{quota}</Badge>
                  {label ? (
                    <Badge variant={preferenceClass === "avoided" ? "destructive" : "default"}>
                      {label}
                    </Badge>
                  ) : null}
                </div>
                <div className="grid min-w-0 gap-4 md:grid-cols-2">
                  <DynamicPolicyEditor
                    errorClass="transient"
                    policy={candidate.policies.transient}
                    onChange={(patch) => updateCandidatePolicy(index, "transient", patch)}
                  />
                  <DynamicPolicyEditor
                    errorClass="hard"
                    policy={candidate.policies.hard}
                    onChange={(patch) => updateCandidatePolicy(index, "hard", patch)}
                  />
                </div>
              </li>
            );
          })}
        </ol>
      )}
    </div>
  );
}
