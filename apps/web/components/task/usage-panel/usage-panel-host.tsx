"use client";

import { useAppStore } from "@/components/state-provider";
import { UsagePanel } from "./usage-panel";

/**
 * Binds the Usage panel to the active task, the way the prompt-history host
 * binds to the active session. Both the desktop dockview registry and the
 * mobile session layout render this.
 */
export function UsagePanelHost() {
  const taskId = useAppStore((state) => state.tasks.activeTaskId);
  return <UsagePanel taskId={taskId ?? null} />;
}
