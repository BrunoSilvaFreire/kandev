"use client";

import { Badge } from "@kandev/ui/badge";

/**
 * Heading of one outer board group. The group is shown once for the whole
 * board; the workflows that hold its tasks render beneath it.
 */
export function KanbanGlobalGroupHeader({ label, count }: { label: string; count: number }) {
  return (
    <div
      className="flex items-center gap-2 border-b border-border pb-1"
      data-testid="kanban-global-group-header"
    >
      <h2 className="truncate text-sm font-semibold text-foreground">{label}</h2>
      <Badge variant="secondary" className="shrink-0">
        {count}
      </Badge>
    </div>
  );
}
