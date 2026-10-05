import { Progress } from "@kandev/ui/progress";
import { useTranslation } from "react-i18next";
import type { ProviderUsageIndexStatus } from "@/lib/types/provider-usage";

type Props = {
  status: ProviderUsageIndexStatus | null;
};

function progressPct(status: ProviderUsageIndexStatus): number {
  if (status.files_total <= 0) return 0;
  return Math.min(100, Math.round((status.files_done / status.files_total) * 100));
}

/** Shows index progress while a run is active, and a failure line if one failed. */
export function UsageIndexBanner({ status }: Props) {
  const { t } = useTranslation();
  if (!status || status.state === "done" || status.state === "never") return null;

  if (status.state === "failed") {
    return (
      <div role="status" className="rounded-lg border border-amber-500/40 p-3 text-sm">
        {t("usage:indexFailed")}
      </div>
    );
  }

  return (
    <div role="status" className="space-y-2 rounded-lg border p-3 text-sm">
      <p>{t("usage:indexRunning", { done: status.files_done, total: status.files_total })}</p>
      <Progress value={progressPct(status)} />
    </div>
  );
}
