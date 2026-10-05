"use client";

import { useTranslation } from "react-i18next";
import { SUPPORTED_GROUPS } from "@/lib/view-model";
import type { ThreadView } from "@/lib/state/slices/ui/thread-view-types";
import { ViewGroupControl } from "@/components/view-model/view-group-control";
import { SectionLabel } from "./threads-view-editor-actions";

/** The Threads view's grouping picker, backed by the shared group model. */
export function ThreadGroupSection({
  group,
  mobile,
  onChange,
}: {
  group: ThreadView["group"];
  mobile: boolean;
  onChange: (group: ThreadView["group"]) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-2 border-b p-2">
      <SectionLabel>{t("task:groupBy")}</SectionLabel>
      <div className={mobile ? "[&_button]:min-h-11" : undefined}>
        <ViewGroupControl
          value={group}
          groups={SUPPORTED_GROUPS.threads}
          onChange={(next) => onChange(next as ThreadView["group"])}
          testId="threads-group-select"
        />
      </div>
    </div>
  );
}
