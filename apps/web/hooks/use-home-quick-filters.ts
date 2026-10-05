"use client";

import { useCallback } from "react";
import { useAppStore } from "@/components/state-provider";
import { updateUserSettingsWithRetry } from "@/lib/user-settings-sync";
import { resolveQuickFilterDimensions } from "@/lib/view-model/quick-filters";
import type { HomeViewId } from "@/lib/view-model/types";

/**
 * Read and write the Quick Filter configuration for one Home view
 * (`userSettings.homeQuickFilters`). Writes update the store optimistically and
 * persist through the shared user-settings sync path.
 */
export function useHomeQuickFilters(view: HomeViewId) {
  const settings = useAppStore((state) => state.userSettings);
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  const configured = settings?.homeQuickFilters?.[view];
  const dimensions = resolveQuickFilterDimensions(view, configured);

  const persist = useCallback(
    (next: string[]) => {
      if (!settings) return;
      const homeQuickFilters = { ...(settings.homeQuickFilters ?? {}), [view]: next };
      setUserSettings?.({ ...settings, homeQuickFilters });
      void updateUserSettingsWithRetry({ home_quick_filters: homeQuickFilters }).catch(
        () => undefined,
      );
    },
    [settings, setUserSettings, view],
  );

  const toggleDimension = useCallback(
    (dimension: string, visible: boolean) => {
      const current = resolveQuickFilterDimensions(view, settings?.homeQuickFilters?.[view]);
      const next = visible
        ? [...current.filter((value) => value !== dimension), dimension]
        : current.filter((value) => value !== dimension);
      persist(next);
    },
    [persist, settings?.homeQuickFilters, view],
  );

  return { dimensions, setDimensions: persist, toggleDimension };
}
