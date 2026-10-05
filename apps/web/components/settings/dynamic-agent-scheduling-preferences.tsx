"use client";

import { useTranslation } from "react-i18next";
import {
  SettingsFieldDescription,
  SettingsFieldLabel,
} from "@/components/settings/settings-typography";
import { TagTokenInput } from "@/components/settings/tag-token-input";

type DynamicAgentSchedulingPreferencesProps = {
  preferredTags: string[];
  avoidedTags: string[];
  onPreferredChange: (tags: string[]) => void;
  onAvoidedChange: (tags: string[]) => void;
  conflict: string | null;
};

/**
 * Soft preferred/avoided tag lists for one dynamic profile. Both lists are
 * matched against each candidate's concrete profile tags; they never discover
 * candidates and never override capacity or circuit health.
 */
export function DynamicAgentSchedulingPreferences({
  preferredTags,
  avoidedTags,
  onPreferredChange,
  onAvoidedChange,
  conflict,
}: DynamicAgentSchedulingPreferencesProps) {
  const { t } = useTranslation();
  return (
    <section className="min-w-0 space-y-4" data-testid="dynamic-profile-preferences">
      <div>
        <h3 className="text-sm font-medium">{t("agents:dynamicPreferencesTitle")}</h3>
        <p className="text-xs text-muted-foreground">{t("agents:dynamicPreferencesDescription")}</p>
      </div>
      <div className="space-y-2">
        <SettingsFieldLabel htmlFor="dynamic-preferred-tags">
          {t("agents:dynamicPreferredTagsLabel")}
        </SettingsFieldLabel>
        <TagTokenInput
          id="dynamic-preferred-tags"
          tags={preferredTags}
          onChange={onPreferredChange}
          placeholder={t("agents:dynamicPreferredTagsPlaceholder")}
          removeLabel={(tag) => t("agents:dynamicPreferenceRemove", { tag })}
          listTestId="dynamic-preferred-tags-list"
          inputTestId="dynamic-preferred-tags-input"
        />
      </div>
      <div className="space-y-2">
        <SettingsFieldLabel htmlFor="dynamic-avoided-tags">
          {t("agents:dynamicAvoidedTagsLabel")}
        </SettingsFieldLabel>
        <TagTokenInput
          id="dynamic-avoided-tags"
          tags={avoidedTags}
          onChange={onAvoidedChange}
          placeholder={t("agents:dynamicAvoidedTagsPlaceholder")}
          removeLabel={(tag) => t("agents:dynamicPreferenceRemove", { tag })}
          listTestId="dynamic-avoided-tags-list"
          inputTestId="dynamic-avoided-tags-input"
        />
      </div>
      {conflict ? (
        <p
          className="text-xs text-destructive"
          role="alert"
          data-testid="dynamic-preference-conflict"
        >
          {t("agents:dynamicPreferenceConflict", { tag: conflict })}
        </p>
      ) : null}
      <SettingsFieldDescription>{t("agents:dynamicPreferencesSoftHelp")}</SettingsFieldDescription>
    </section>
  );
}
