"use client";

import { Card } from "@kandev/ui/card";
import { Switch } from "@kandev/ui/switch";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import type { ProviderUsageSource } from "@/lib/types/provider-usage";

export const TOGGLEABLE_SOURCE_IDS = ["claude_local", "codex_local", "antigravity_local"] as const;

export type ToggleableSourceId = (typeof TOGGLEABLE_SOURCE_IDS)[number];

function sourceLabel(t: TFunction, source: string): string {
  if (source === "claude_local") return t("usage:sourceClaude");
  if (source === "codex_local") return t("usage:sourceCodex");
  if (source === "antigravity_local") return t("usage:sourceAntigravity");
  return source;
}

function sourceDetail(t: TFunction, source: string): string {
  if (source === "claude_local") return t("usage:sourceClaudeDetail");
  if (source === "codex_local") return t("usage:sourceCodexDetail");
  if (source === "antigravity_local") return t("usage:sourceAntigravityDetail");
  return "";
}

type Props = {
  sources: ProviderUsageSource[];
  onChange: (source: ToggleableSourceId, enabled: boolean) => void;
};

/**
 * History sources card. Kandev sessions are always included; the three local
 * sources are opt-in and explain what they read.
 */
export function UsageSourcesCard({ sources, onChange }: Props) {
  const { t } = useTranslation();
  const toggleable = sources.filter((source) => source.toggleable);

  return (
    <Card className="space-y-3 p-4" data-testid="usage-sources-card">
      <div>
        <h2 className="text-sm font-medium">{t("usage:sourcesTitle")}</h2>
        <p className="text-xs text-muted-foreground">{t("usage:sourcesDescription")}</p>
      </div>
      <div className="flex items-center justify-between gap-3">
        <div>
          <p className="text-sm">{t("usage:sourcesKandev")}</p>
          <p className="text-xs text-muted-foreground">{t("usage:sourcesKandevDetail")}</p>
        </div>
        <Switch checked disabled aria-label={t("usage:sourcesKandev")} />
      </div>
      {toggleable.map((source) => (
        <div key={source.source} className="flex items-center justify-between gap-3">
          <div>
            <p className="text-sm">{sourceLabel(t, source.source)}</p>
            <p className="text-xs text-muted-foreground">{sourceDetail(t, source.source)}</p>
            <p className="text-xs text-muted-foreground">
              {source.enabled ? t("usage:sourceDisableHint") : t("usage:sourceEnableHint")}
            </p>
          </div>
          <Switch
            checked={source.enabled}
            aria-label={sourceLabel(t, source.source)}
            onCheckedChange={(value) => onChange(source.source as ToggleableSourceId, value)}
          />
        </div>
      ))}
    </Card>
  );
}
