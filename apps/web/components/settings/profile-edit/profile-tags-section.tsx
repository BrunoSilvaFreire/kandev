"use client";

import { useTranslation } from "react-i18next";
import { CardContent } from "@kandev/ui/card";
import { SettingsCard } from "@/components/settings/settings-card";
import { SettingsCardHeader } from "@/components/settings/settings-card-header";
import {
  SettingsFieldDescription,
  SettingsFieldLabel,
} from "@/components/settings/settings-typography";
import {
  TagTokenInput,
  areTagListsEqual,
} from "@/components/settings/tag-token-input";

type ProfileTagsSectionProps = {
  tags?: string[];
  baselineTags?: string[];
  onChange: (patch: { tags: string[] }) => void;
  discoveryTargetId?: string;
};

export function ProfileTagsSection({
  tags,
  baselineTags,
  onChange,
  discoveryTargetId,
}: ProfileTagsSectionProps) {
  const { t } = useTranslation();
  const current = tags ?? [];

  return (
    <SettingsCard
      isDirty={!areTagListsEqual(current, baselineTags)}
      discoveryTargetId={discoveryTargetId}
    >
      <SettingsCardHeader
        title={t("agents:profileTagsTitle")}
        description={t("agents:profileTagsDescription")}
      />
      <CardContent className="space-y-2">
        <SettingsFieldLabel>{t("agents:profileTagsLabel")}</SettingsFieldLabel>
        <TagTokenInput
          tags={current}
          onChange={(next) => onChange({ tags: next })}
          placeholder={t("agents:profileTagsPlaceholder")}
          removeLabel={(tag) => t("agents:profileTagsRemove", { tag })}
          listTestId="profile-tags-list"
          inputTestId="profile-tags-input"
        />
        <SettingsFieldDescription>{t("agents:profileTagsHelp")}</SettingsFieldDescription>
      </CardContent>
    </SettingsCard>
  );
}
