"use client";

import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import { isDynamicErrorPolicyValid } from "@/components/settings/dynamic-agent-policy-editor";
import { updateAgentProfileAction } from "@/app/actions/agents";
import { isHandledApiError } from "@/lib/api/client";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { toAgentProfileOption } from "@/lib/state/slices/settings/types";
import type { Agent, AgentProfile } from "@/lib/types/http";
import type {
  DynamicAgentCandidate,
  DynamicErrorClass,
  DynamicErrorPolicy,
} from "@/lib/types/agent-profile";
import {
  dynamicDraftRevision,
  useDynamicAgentProfileEditorDraft,
} from "@/components/settings/dynamic-agent-profile-editor-draft";

type DynamicAgentProfileEditorStateProps = {
  agent: Agent;
  profile: AgentProfile;
  onDraftChange?: (patch: Pick<AgentProfile, "name" | "dynamic" | "enabled">) => void;
};

export type DynamicAgentProfileEditorState = {
  name: string;
  profileEnabled: boolean;
  standalone: boolean;
  hasExternalConflict: boolean;
  routingEnabled: boolean;
  enabledLabel: string;
  concreteProfiles: AgentProfile[];
  availableProfileOptions: ReturnType<typeof toAgentProfileOption>[];
  updateName: (name: string) => void;
  updateProfileEnabled: (enabled: boolean) => void;
  updatePreferredTags: (tags: string[]) => void;
  updateAvoidedTags: (tags: string[]) => void;
  preferredTags: string[];
  avoidedTags: string[];
  preferenceConflict: string | null;
  addCandidate: (executionProfileId: string) => void;
  moveCandidate: (index: number, direction: -1 | 1) => void;
  removeCandidate: (index: number) => void;
  updateCandidate: (index: number, patch: Partial<DynamicAgentCandidate>) => void;
  updateCandidatePolicy: (
    index: number,
    errorClass: DynamicErrorClass,
    patch: Partial<DynamicErrorPolicy>,
  ) => void;
  candidates: DynamicAgentCandidate[];
  discardDraft: () => void;
};

export function dynamicTagConflict(preferred: string[], avoided: string[]): string | null {
  const preferredSet = new Set(preferred);
  return avoided.find((tag) => preferredSet.has(tag)) ?? null;
}

type DraftGateInput = {
  routingEnabled: boolean;
  saving: boolean;
  hasExternalConflict: boolean;
  preferenceConflict: string | null;
  name: string;
  candidateCount: number;
  policiesValid: boolean;
};

function canSaveDynamicDraft(input: DraftGateInput): boolean {
  return (
    input.routingEnabled &&
    !input.saving &&
    !input.hasExternalConflict &&
    !input.preferenceConflict &&
    Boolean(input.name.trim()) &&
    input.candidateCount > 0 &&
    input.policiesValid
  );
}

function resolveDraftInvalidReason(input: {
  name: string;
  candidateCount: number;
  preferenceConflict: string | null;
  hasExternalConflict: boolean;
  t: (key: string, options?: Record<string, unknown>) => string;
}): string {
  if (!input.name.trim()) return input.t("agents:profileNameRequired");
  if (input.candidateCount === 0) return input.t("agents:noDynamicCandidates");
  if (input.preferenceConflict)
    return input.t("agents:dynamicPreferenceConflict", { tag: input.preferenceConflict });
  if (input.hasExternalConflict) return input.t("agents:profileExternalChangeInvalidReason");
  return input.t("agents:dynamicPolicyValidation");
}

type DynamicProfilePayloadInput = {
  name: string;
  enabled: boolean;
  version: number;
  preferredTags: string[];
  avoidedTags: string[];
  candidates: DynamicAgentCandidate[];
};

export function dynamicProfilePayload(input: DynamicProfilePayloadInput) {
  const { name, enabled, version, preferredTags, avoidedTags, candidates } = input;
  return {
    name: name.trim(),
    enabled,
    dynamic: {
      version,
      preferred_tags: preferredTags,
      avoided_tags: avoidedTags,
      candidates: candidates.map((candidate, position) => ({
        position,
        execution_profile_id: candidate.executionProfileId,
        enabled: candidate.enabled,
        policies: {
          version: candidate.policies.version,
          transient: {
            retry: {
              enabled: candidate.policies.transient.retry.enabled,
              max_retries: candidate.policies.transient.retry.maxRetries,
              initial_interval_seconds: candidate.policies.transient.retry.initialIntervalSeconds,
            },
            wait_for_reset: {
              enabled: candidate.policies.transient.waitForReset.enabled,
              max_wait_seconds: candidate.policies.transient.waitForReset.maxWaitSeconds,
            },
            on_exhausted: candidate.policies.transient.onExhausted,
          },
          hard: {
            retry: {
              enabled: candidate.policies.hard.retry.enabled,
              max_retries: candidate.policies.hard.retry.maxRetries,
              initial_interval_seconds: candidate.policies.hard.retry.initialIntervalSeconds,
            },
            wait_for_reset: {
              enabled: candidate.policies.hard.waitForReset.enabled,
              max_wait_seconds: candidate.policies.hard.waitForReset.maxWaitSeconds,
            },
            on_exhausted: candidate.policies.hard.onExhausted,
          },
        },
      })),
    },
  };
}

// eslint-disable-next-line max-lines-per-function -- coordinates persistence and the shared save surface.
export function useDynamicAgentProfileEditorState({
  agent,
  profile,
  onDraftChange,
}: DynamicAgentProfileEditorStateProps): DynamicAgentProfileEditorState {
  const { t } = useTranslation();
  const enabledLabel = t("agents:enabled");
  const { toast } = useToast();
  const routingEnabled = useFeature("dynamicAgentRouting");
  const settingsAgents = useAppStore((state) => state.settingsAgents.items);
  const setSettingsAgents = useAppStore((state) => state.setSettingsAgents);
  const setAgentProfiles = useAppStore((state) => state.setAgentProfiles);
  const draft = useDynamicAgentProfileEditorDraft({ profile, onDraftChange });
  const [saving, setSaving] = useState(false);
  const standalone = onDraftChange === undefined;

  const concreteProfiles = useMemo(
    () =>
      settingsAgents
        .filter((item) => item.name !== "dynamic")
        .flatMap((item) => item.profiles)
        .filter(
          (candidate) =>
            candidate.kind !== "dynamic" && candidate.enabled !== false && !candidate.workspaceId,
        ),
    [settingsAgents],
  );
  const availableProfileOptions = useMemo(
    () =>
      settingsAgents.flatMap((item) =>
        item.name === "dynamic"
          ? []
          : item.profiles
              .filter(
                (candidate) =>
                  candidate.kind !== "dynamic" &&
                  candidate.enabled !== false &&
                  !candidate.workspaceId &&
                  !draft.candidates.some((current) => current.executionProfileId === candidate.id),
              )
              .map((candidate) => toAgentProfileOption(item, candidate)),
      ),
    [draft.candidates, settingsAgents],
  );

  const preferenceConflict = dynamicTagConflict(draft.preferredTags, draft.avoidedTags);
  const save = async () => {
    if (
      !profile.dynamic ||
      !canSaveDynamicDraft({
        routingEnabled,
        saving: false,
        hasExternalConflict: draft.hasExternalConflict,
        preferenceConflict,
        name: draft.name,
        candidateCount: draft.candidates.length,
        policiesValid: true,
      })
    ) {
      return;
    }
    setSaving(true);
    const submitted = draft.currentProfile;
    draft.markProfileSubmitted(submitted);
    try {
      const draftPayload = {
        name: draft.name.trim(),
        enabled: draft.profileEnabled,
        dynamic: submitted.dynamic,
      };
      const payload = dynamicProfilePayload({
        name: draft.name,
        enabled: draft.profileEnabled,
        version: draft.dynamicVersion,
        preferredTags: draft.preferredTags,
        avoidedTags: draft.avoidedTags,
        candidates: draft.candidates,
      });
      if (onDraftChange) {
        onDraftChange(draftPayload);
        return;
      }
      const updated = await updateAgentProfileAction(profile.id, payload);
      const nextAgents = settingsAgents.map((item) =>
        item.id !== agent.id
          ? item
          : {
              ...item,
              profiles: item.profiles.map((itemProfile) =>
                itemProfile.id === updated.id ? updated : itemProfile,
              ),
            },
      );
      setSettingsAgents(nextAgents);
      setAgentProfiles(
        nextAgents.flatMap((item) =>
          item.profiles.map((itemProfile) => toAgentProfileOption(item, itemProfile)),
        ),
      );
      draft.acceptProfileSaveResponse(updated, submitted);
      toast({ title: t("agents:dynamicProfileSaved") });
    } catch (error) {
      draft.markProfileSubmitted(null);
      if (isHandledApiError(error)) return;
      toast({
        title: t("agents:failedToSaveProfile"),
        description: error instanceof Error ? error.message : undefined,
        variant: "error",
      });
    } finally {
      setSaving(false);
    }
  };

  const draftRevision = dynamicDraftRevision(
    draft.name,
    draft.preferredTags,
    draft.avoidedTags,
    draft.candidates,
    draft.profileEnabled,
  );
  const savedRevision = dynamicDraftRevision(
    draft.savedProfile.name,
    draft.savedProfile.dynamic?.preferredTags ?? [],
    draft.savedProfile.dynamic?.avoidedTags ?? [],
    draft.savedProfile.dynamic?.candidates ?? [],
    draft.savedProfile.enabled !== false,
  );
  const policiesValid = draft.candidates.every(
    (candidate) =>
      isDynamicErrorPolicyValid(candidate.policies.transient) &&
      isDynamicErrorPolicyValid(candidate.policies.hard),
  );
  const invalidReason = resolveDraftInvalidReason({
    name: draft.name,
    candidateCount: draft.candidates.length,
    preferenceConflict,
    hasExternalConflict: draft.hasExternalConflict,
    t,
  });
  useSettingsSaveContributor({
    id: `dynamic-profile:${profile.id}`,
    revision: draftRevision,
    isDirty: standalone && draftRevision !== savedRevision,
    canSave: canSaveDynamicDraft({
      routingEnabled,
      saving,
      hasExternalConflict: draft.hasExternalConflict,
      preferenceConflict,
      name: draft.name,
      candidateCount: draft.candidates.length,
      policiesValid,
    }),
    invalidReason,
    save,
    discard: () => {
      if (!standalone) return;
      draft.reset();
    },
  });

  return {
    name: draft.name,
    profileEnabled: draft.profileEnabled,
    standalone,
    hasExternalConflict: draft.hasExternalConflict,
    routingEnabled,
    enabledLabel,
    concreteProfiles,
    availableProfileOptions,
    updateName: draft.updateName,
    updateProfileEnabled: draft.updateProfileEnabled,
    updatePreferredTags: draft.updatePreferredTags,
    updateAvoidedTags: draft.updateAvoidedTags,
    preferredTags: draft.preferredTags,
    avoidedTags: draft.avoidedTags,
    preferenceConflict,
    addCandidate: draft.addCandidate,
    moveCandidate: draft.moveCandidate,
    removeCandidate: draft.removeCandidate,
    updateCandidate: draft.updateCandidate,
    updateCandidatePolicy: draft.updateCandidatePolicy,
    candidates: draft.candidates,
    discardDraft: draft.reset,
  };
}
