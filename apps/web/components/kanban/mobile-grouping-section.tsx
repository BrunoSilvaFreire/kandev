"use client";

import { useTranslation } from "react-i18next";
import { KanbanGroupingAndPresets } from "./kanban-view-filters";
import { mobileSectionClass, mobileSectionTitleClass } from "./mobile-menu-styles";

/**
 * Phone counterpart of the desktop header's grouping picker and presets: the
 * same controls, stacked and touch-sized inside the listing menu.
 */
export function MobileGroupingSection({ view }: { view: "kanban" | "list" }) {
  const { t } = useTranslation();
  return (
    <div className={mobileSectionClass} data-testid="mobile-grouping-section">
      <span className={mobileSectionTitleClass}>{t("task:groupingSection")}</span>
      <div className="flex flex-col gap-2">
        <KanbanGroupingAndPresets view={view} />
      </div>
    </div>
  );
}
