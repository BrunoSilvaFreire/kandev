"use client";

import { useTranslation } from "react-i18next";
import { CardContent } from "@kandev/ui/card";
import { SettingsCard } from "@/components/settings/settings-card";
import { SettingsCardHeader } from "@/components/settings/settings-card-header";
import { UsageCredentialButton } from "@/components/usage/usage-credential-dialog";
import { useUsageCredentials } from "@/hooks/domains/usage/use-usage-credentials";

/**
 * Provider-credential card for agents whose live quota needs an extra secret.
 * The dialog saves immediately, so this card has no unsaved local state and
 * registers no save contributor.
 */
export function UsageCredentialCard({ agentType }: { agentType: string }) {
  const { t } = useTranslation();
  const { hint, refresh } = useUsageCredentials(agentType);
  if (!hint) return null;
  return (
    <SettingsCard data-testid="usage-credential-card">
      <SettingsCardHeader
        title={t("usage:credentialCardTitle")}
        description={t("usage:credentialCardDescription")}
        actions={<UsageCredentialButton hint={hint} onSaved={refresh} />}
      />
      <CardContent className="space-y-1">
        <p className="text-sm" data-testid="usage-credential-state">
          {hint.configured ? t("usage:credentialConfigured") : t("usage:credentialNotConfigured")}
        </p>
        <p className="text-xs text-muted-foreground">{t("usage:credentialSavedImmediately")}</p>
      </CardContent>
    </SettingsCard>
  );
}
