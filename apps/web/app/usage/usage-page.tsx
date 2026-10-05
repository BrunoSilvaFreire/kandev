"use client";

import { useState } from "react";
import { Button } from "@kandev/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@kandev/ui/tabs";
import { IconGauge } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { usePathname, useRouter, useSearchParams } from "@/lib/routing/client-router";
import type { ProviderUsageRange } from "@/lib/types/provider-usage";
import { PageShell } from "@/components/page-shell";
import { UsageBreakdownTab } from "@/components/usage/usage-breakdown-tab";
import { UsageQuotasTab } from "@/components/usage/usage-quotas-tab";

const RANGES: ProviderUsageRange[] = ["24h", "7d", "30d"];
const QUOTAS_TAB = "quotas";
const BREAKDOWN_TAB = "breakdown";

function rangeLabelKey(range: ProviderUsageRange): string {
  if (range === "24h") return "usage:range24h";
  if (range === "30d") return "usage:range30d";
  return "usage:range7d";
}

function RangeSelector({
  value,
  onChange,
}: {
  value: ProviderUsageRange;
  onChange: (range: ProviderUsageRange) => void;
}) {
  const { t } = useTranslation();
  return (
    <div
      className="flex items-center gap-1 rounded-md border p-0.5"
      role="group"
      aria-label={t("usage:rangeLabel")}
    >
      {RANGES.map((range) => (
        <Button
          key={range}
          variant={range === value ? "secondary" : "ghost"}
          size="sm"
          onClick={() => onChange(range)}
          className="h-7 cursor-pointer px-2 text-xs"
        >
          {t(rangeLabelKey(range))}
        </Button>
      ))}
    </div>
  );
}

/**
 * Top-level provider usage page: quota cards on the Quotas tab and a grouped
 * ledger explorer on the Breakdown tab. Both share the topbar range selector.
 */
export function UsagePageClient() {
  const { t } = useTranslation();
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const [range, setRange] = useState<ProviderUsageRange>("7d");
  const [refreshToken, setRefreshToken] = useState(0);

  const activeTab = searchParams.get("tab") === BREAKDOWN_TAB ? BREAKDOWN_TAB : QUOTAS_TAB;
  const handleTabChange = (value: string) => {
    const params = new URLSearchParams(searchParams);
    if (value === QUOTAS_TAB) params.delete("tab");
    else params.set("tab", value);
    const query = params.toString();
    router.replace(query ? `${pathname}?${query}` : pathname);
  };

  return (
    <PageShell
      title={t("usage:title")}
      subtitle={t("usage:subtitle")}
      icon={<IconGauge className="h-4 w-4" />}
      topbarTestId="usage-topbar"
      actions={
        <div className="flex items-center gap-2">
          <RangeSelector value={range} onChange={setRange} />
          <Button
            variant="outline"
            size="sm"
            onClick={() => setRefreshToken((token) => token + 1)}
            className="cursor-pointer"
          >
            {t("usage:refresh")}
          </Button>
        </div>
      }
    >
      <div className="mx-auto w-full max-w-5xl space-y-4 p-4 md:p-6">
        <Tabs value={activeTab} onValueChange={handleTabChange} data-testid="usage-tabs">
          <TabsList>
            <TabsTrigger
              value={QUOTAS_TAB}
              data-testid="usage-tab-quotas"
              className="cursor-pointer"
            >
              {t("usage:tabQuotas")}
            </TabsTrigger>
            <TabsTrigger
              value={BREAKDOWN_TAB}
              data-testid="usage-tab-breakdown"
              className="cursor-pointer"
            >
              {t("usage:tabBreakdown")}
            </TabsTrigger>
          </TabsList>
          <TabsContent value={QUOTAS_TAB} className="pt-4">
            <UsageQuotasTab range={range} refreshToken={refreshToken} />
          </TabsContent>
          <TabsContent value={BREAKDOWN_TAB} className="pt-4">
            <UsageBreakdownTab range={range} refreshToken={refreshToken} />
          </TabsContent>
        </Tabs>
      </div>
    </PageShell>
  );
}
