"use client";

import { Badge } from "@kandev/ui/badge";
import { Card } from "@kandev/ui/card";
import { useTranslation } from "react-i18next";
import type {
  ProviderUsageAccount,
  ProviderUsageProvider,
  ProviderUsageWindow,
} from "@/lib/types/provider-usage";
import { UsageAccountFacts } from "./usage-account-facts";
import { UsageCredentialButton } from "./usage-credential-dialog";
import { UsageInfoChip } from "./usage-info-chip";
import { UsageSparkline } from "./usage-sparkline";
import { UsageStatusChip } from "./usage-status-chip";
import {
  clampPct,
  estimateChips,
  fetchedChip,
  quotaUnavailableReasonKey,
  resetChip,
  utilizationBarColor,
  utilizationTextColor,
} from "./usage-format";

function WindowRow({ window }: { window: ProviderUsageWindow }) {
  const { t } = useTranslation();
  const provenance =
    window.source === "estimated" ? t("usage:provenanceEstimated") : t("usage:provenanceReported");
  const provenanceTooltip =
    window.source === "estimated"
      ? t("usage:provenanceEstimatedTooltip")
      : t("usage:provenanceReportedTooltip");
  const reset = resetChip(t, window.reset_at);
  const fetched = fetchedChip(t, window.history, window.source);
  return (
    <div className="space-y-2" data-testid="usage-window">
      <div className="flex items-center justify-between text-xs">
        <span className="text-muted-foreground capitalize">{window.label}</span>
        <span className={`font-medium ${utilizationTextColor(window.utilization_pct)}`}>
          {Math.round(window.utilization_pct)}%
        </span>
      </div>
      <div className="h-2 overflow-hidden rounded-full bg-muted">
        <div
          className={`h-full rounded-full transition-all ${utilizationBarColor(window.utilization_pct)}`}
          style={{ width: `${clampPct(window.utilization_pct)}%` }}
        />
      </div>
      <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
        <UsageInfoChip
          label={provenance}
          tooltip={provenanceTooltip}
          tone={window.source === "estimated" ? "neutral" : "healthy"}
          testId="usage-provenance-badge"
        />
        <UsageInfoChip label={reset.label} tooltip={reset.tooltip} testId="usage-reset-chip" />
        {fetched && (
          <UsageInfoChip
            label={fetched.label}
            tooltip={fetched.tooltip}
            testId="usage-fetched-chip"
          />
        )}
        {estimateChips(t, window.estimate).map((chip) => (
          <UsageInfoChip
            key={chip.key}
            label={chip.label}
            tooltip={chip.tooltip}
            tone={chip.tone}
            testId={`usage-estimate-chip-${chip.key}`}
          />
        ))}
        {window.estimate?.anomaly && (
          <UsageInfoChip
            label={t("usage:anomaly")}
            tooltip={t("usage:anomalyTooltip")}
            tone="nearing"
            testId="usage-anomaly-chip"
          />
        )}
      </div>
      <UsageSparkline points={window.history} />
    </div>
  );
}

function AccountBlock({
  account,
  onCredentialSaved,
}: {
  account: ProviderUsageAccount;
  onCredentialSaved?: () => void;
}) {
  const { t } = useTranslation();
  const names = account.profiles.map((profile) => profile.name).join(", ");
  return (
    <div className="space-y-3 border-t pt-3" data-testid="usage-account">
      <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
        <UsageStatusChip status={account.status} />
        {account.label && <span className="font-medium text-foreground">{account.label}</span>}
        {account.plan && !account.label && <Badge variant="secondary">{account.plan}</Badge>}
        {account.profiles.length > 0 && <span>{t("usage:profilesUsing", { names })}</span>}
        {account.credential && (
          <UsageCredentialButton hint={account.credential} onSaved={onCredentialSaved} />
        )}
      </div>
      <UsageAccountFacts account={account} />
      {account.windows.length > 0 &&
        account.windows.map((window) => <WindowRow key={window.label} window={window} />)}
      {account.windows.length === 0 && account.status === "unavailable" && (
        <p className="text-xs text-muted-foreground">{t("usage:sourceNotRunning")}</p>
      )}
      {account.windows.length === 0 && account.status === "unknown" && (
        <p className="text-xs text-muted-foreground">
          {account.quota_unavailable_reason
            ? t(quotaUnavailableReasonKey(account.quota_unavailable_reason))
            : t("usage:noQuotaSource")}
        </p>
      )}
      {account.windows.length === 0 &&
        account.status !== "unavailable" &&
        account.status !== "unknown" && (
          <p className="text-xs text-muted-foreground">{t("usage:noRecentData")}</p>
        )}
    </div>
  );
}

/** One provider card with its deduplicated accounts. */
export function UsageProviderCard({
  provider,
  onCredentialSaved,
}: {
  provider: ProviderUsageProvider;
  onCredentialSaved?: () => void;
}) {
  return (
    <Card className="space-y-4 p-4" data-testid="usage-provider-card">
      <div className="flex items-center justify-between">
        <h2 className="text-sm font-medium">{provider.display_name ?? provider.provider}</h2>
        <UsageStatusChip status={provider.status} />
      </div>
      {provider.accounts.map((account) => (
        <AccountBlock
          key={account.account_key}
          account={account}
          onCredentialSaved={onCredentialSaved}
        />
      ))}
    </Card>
  );
}
