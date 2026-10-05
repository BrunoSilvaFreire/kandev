"use client";

import { useEffect } from "react";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";
import { useProviderUsage } from "@/hooks/domains/usage/use-provider-usage";
import { useUsageIndex } from "@/hooks/domains/usage/use-usage-index";
import { updateUsageSources } from "@/lib/api/domains/provider-usage-api";
import { setProviderUsageAttention } from "@/lib/state/provider-usage-attention";
import type { ProviderUsageRange } from "@/lib/types/provider-usage";
import { UsageIndexBanner } from "./usage-index-banner";
import { UsageProviderCard } from "./usage-provider-card";
import { UsageSourcesCard, type ToggleableSourceId } from "./usage-sources-card";

/** The existing quota view: index status, sources, and one card per provider. */
export function UsageQuotasTab({
  range,
  refreshToken,
}: {
  range: ProviderUsageRange;
  refreshToken: number;
}) {
  const { t } = useTranslation();
  const { overview, loading, error, refresh } = useProviderUsage(range, refreshToken);
  // Refetch the overview when a run ends so backfilled history appears without
  // a manual Refresh.
  const { status, reindex, refresh: refreshIndex } = useUsageIndex({ onFinished: refresh });

  // The sidebar reads this flag; it never polls itself. It is deliberately not
  // cleared on unmount: it reflects the last fetched overview until a newer one
  // replaces it.
  useEffect(() => {
    const attention =
      overview?.providers.some(
        (provider) => provider.status === "nearing" || provider.status === "exhausted",
      ) ?? false;
    setProviderUsageAttention(attention);
  }, [overview]);

  const handleSourceChange = async (source: ToggleableSourceId, enabled: boolean) => {
    if (!enabled && !window.confirm(t("usage:confirmDisable"))) return;
    try {
      await updateUsageSources({ [source]: enabled });
      refresh();
      refreshIndex();
    } catch {
      // The switch reflects the last server-confirmed state on the next read.
    }
  };

  return (
    <div className="space-y-4">
      <UsageIndexBanner status={status ?? overview?.indexing ?? null} />

      {overview && <UsageSourcesCard sources={overview.sources} onChange={handleSourceChange} />}

      {loading && (
        <p role="status" className="text-sm text-muted-foreground">
          {t("usage:loading")}
        </p>
      )}
      {error && <p className="text-sm text-muted-foreground">{t("usage:loadError")}</p>}
      {overview?.providers.map((provider) => (
        <UsageProviderCard
          key={provider.provider}
          provider={provider}
          onCredentialSaved={refresh}
        />
      ))}
      {overview && overview.providers.length === 0 && !loading && (
        <p className="text-sm text-muted-foreground">{t("usage:empty")}</p>
      )}

      <div className="flex justify-end">
        <Button variant="ghost" size="sm" onClick={reindex} className="cursor-pointer">
          {t("usage:reindex")}
        </Button>
      </div>
    </div>
  );
}
