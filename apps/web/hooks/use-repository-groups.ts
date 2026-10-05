"use client";

import { useMemo } from "react";
import { useAppStore } from "@/components/state-provider";
import { toRepositoryGroups } from "@/lib/view-model/repository-group-source";
import type { RepositoryGroup } from "@/lib/view-model/repository-group";

/**
 * The active workspace's Repository Groups, projected for the shared view
 * model. Reads the reactive `repositorySets` slice so it stays live on the
 * existing WebSocket updates.
 */
export function useRepositoryGroups(): RepositoryGroup[] {
  const sets = useAppStore(
    (state) => state.repositorySets?.itemsByWorkspaceId?.[state.workspaces?.activeId ?? ""],
  );
  return useMemo(() => toRepositoryGroups(sets), [sets]);
}
