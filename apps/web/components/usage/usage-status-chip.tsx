import { Badge } from "@kandev/ui/badge";
import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";
import type { ProviderUsageStatus } from "@/lib/types/provider-usage";

const STATUS_STYLE: Record<ProviderUsageStatus, string> = {
  exhausted: "border-red-500/40 text-red-600 dark:text-red-400",
  nearing: "border-amber-500/40 text-amber-600 dark:text-amber-400",
  stale: "border-muted-foreground/40 text-muted-foreground",
  healthy: "border-emerald-500/40 text-emerald-600 dark:text-emerald-400",
  unknown: "border-muted-foreground/40 text-muted-foreground",
  unavailable: "border-muted-foreground/40 text-muted-foreground",
};

/** Status is a translation key, never compared after translation. */
function statusLabelKey(status: ProviderUsageStatus): string {
  if (status === "exhausted") return "usage:statusExhausted";
  if (status === "nearing") return "usage:statusNearing";
  if (status === "stale") return "usage:statusStale";
  if (status === "healthy") return "usage:statusHealthy";
  if (status === "unknown") return "usage:statusUnknown";
  return "usage:statusUnavailable";
}

export function UsageStatusChip({ status }: { status: ProviderUsageStatus }) {
  const { t } = useTranslation();
  return (
    <Badge variant="outline" className={cn("text-xs", STATUS_STYLE[status])}>
      {t(statusLabelKey(status))}
    </Badge>
  );
}
