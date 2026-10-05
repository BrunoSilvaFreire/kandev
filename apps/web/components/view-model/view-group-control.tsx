"use client";

import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { useTranslation } from "react-i18next";
import type { ViewGroupKey } from "@/lib/view-model/types";

// `labelKey` / `descriptionKey` hold catalog keys, not copy: this is module
// scope, so a resolved `t()` here would freeze at the boot locale. `key` is a
// persisted grouping value and stays English.
const GROUP_META: Record<ViewGroupKey, { labelKey: string; descriptionKey: string }> = {
  none: { labelKey: "task:groupNone", descriptionKey: "task:groupNoneDescription" },
  repository: {
    labelKey: "task:groupRepository",
    descriptionKey: "task:groupRepositoryDescription",
  },
  repositoryGroup: {
    labelKey: "task:groupRepositoryGroup",
    descriptionKey: "task:groupRepositoryGroupDescription",
  },
  workflow: { labelKey: "task:groupWorkflow", descriptionKey: "task:groupWorkflowDescription" },
  workflowStep: {
    labelKey: "task:groupWorkflowStep",
    descriptionKey: "task:groupWorkflowStepDescription",
  },
  executorType: {
    labelKey: "task:groupExecutorType",
    descriptionKey: "task:groupExecutorTypeDescription",
  },
  state: { labelKey: "task:groupState", descriptionKey: "task:groupStateDescription" },
  priority: { labelKey: "task:groupPriority", descriptionKey: "task:groupPriorityDescription" },
};

export type ViewGroupControlProps = {
  value: ViewGroupKey;
  /** Group keys the view supports, in display order; must include `value`. */
  groups: readonly ViewGroupKey[];
  onChange: (next: ViewGroupKey) => void;
  testId?: string;
};

/**
 * The one grouping picker every Home view uses. Views pass the subset of group
 * keys they can render; the labels and descriptions come from the shared table.
 */
export function ViewGroupControl({ value, groups, onChange, testId }: ViewGroupControlProps) {
  const { t } = useTranslation();
  return (
    <Select value={value} onValueChange={(next) => onChange(next as ViewGroupKey)}>
      <SelectTrigger className="w-full text-xs" data-testid={testId ?? "view-group-select"}>
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {groups.map((key) => {
          const label = t(GROUP_META[key].labelKey);
          return (
            <SelectItem
              key={key}
              value={key}
              className="text-xs"
              aria-label={label}
              description={t(GROUP_META[key].descriptionKey)}
            >
              {label}
            </SelectItem>
          );
        })}
      </SelectContent>
    </Select>
  );
}

/** The shared display order for group keys a view supports. */
export const VIEW_GROUP_ORDER: readonly ViewGroupKey[] = [
  "none",
  "repository",
  "repositoryGroup",
  "workflow",
  "workflowStep",
  "executorType",
  "state",
  "priority",
];
