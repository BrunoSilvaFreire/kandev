"use client";

import type { GroupKey } from "@/lib/state/slices/ui/sidebar-view-types";
import { VIEW_GROUP_ORDER, ViewGroupControl } from "@/components/view-model/view-group-control";

type Props = {
  value: GroupKey;
  onChange: (next: GroupKey) => void;
};

export function GroupPicker({ value, onChange }: Props) {
  return (
    <ViewGroupControl
      value={value}
      groups={VIEW_GROUP_ORDER}
      onChange={(next) => onChange(next as GroupKey)}
      testId="group-key-select"
    />
  );
}
