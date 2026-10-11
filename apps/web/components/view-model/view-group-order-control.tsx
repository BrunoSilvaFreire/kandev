"use client";

import { useTranslation } from "react-i18next";
import type { ViewGroupKey } from "@/lib/view-model/types";
import { ViewGroupControl } from "./view-group-control";

// ponytail: the control offers two levels; the model and storage accept more.
// Add a row per level when a view needs a third.

export type ViewGroupOrderControlProps = {
  value: readonly ViewGroupKey[];
  /** Group keys the view supports, in display order, including `none`. */
  groups: readonly ViewGroupKey[];
  onChange: (next: ViewGroupKey[]) => void;
  testId: string;
};

/**
 * Ordered grouping picker: the first key forms the outer groups and the
 * second subdivides them. Choosing `none` for the first key clears both.
 */
export function ViewGroupOrderControl({
  value,
  groups,
  onChange,
  testId,
}: ViewGroupOrderControlProps) {
  const { t } = useTranslation();
  const first = value[0] ?? "none";
  const second = value[1] ?? "none";
  const secondOptions = groups.filter((key) => key === "none" || key !== first);
  return (
    <div className="flex items-center gap-2 max-sm:flex-col max-sm:items-stretch">
      <ViewGroupControl
        value={first}
        groups={groups}
        onChange={(next) => onChange(next === "none" ? ["none"] : [next])}
        testId={`${testId}-select`}
        ariaLabel={t("task:groupBy")}
      />
      {first !== "none" && (
        <ViewGroupControl
          value={second}
          groups={secondOptions}
          onChange={(next) => onChange(next === "none" ? [first] : [first, next])}
          testId={`${testId}-then-select`}
          ariaLabel={t("task:groupThenBy")}
        />
      )}
    </div>
  );
}
