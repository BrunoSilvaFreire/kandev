import type { GroupedViewItems, ViewGroup, ViewGroupKey } from "./types";

/** Maps one item to its group key and display label for the active key. */
export type ViewGroupExtractor<Item> = (item: Item) => { key: string; label: string };

/**
 * Group items by the active key. `none` yields one pass-through group. The
 * result preserves first-seen group order, which keeps grouping deterministic
 * for a stable item order.
 */
export function applyViewGroup<Item>(
  items: Item[],
  groupKey: ViewGroupKey,
  extract: ViewGroupExtractor<Item>,
): GroupedViewItems<Item> {
  if (groupKey === "none") {
    return { groups: [{ key: "__all__", label: "", items }], groupKey };
  }
  const buckets = new Map<string, ViewGroup<Item>>();
  for (const item of items) {
    const { key, label } = extract(item);
    const group = buckets.get(key);
    if (group) group.items.push(item);
    else buckets.set(key, { key, label, items: [item] });
  }
  return { groups: [...buckets.values()], groupKey };
}

/** All group keys the shared engine can render, in no particular view order. */
export const VIEW_GROUP_KEYS: readonly ViewGroupKey[] = [
  "none",
  "repository",
  "repositoryGroup",
  "workflow",
  "workflowStep",
  "executorType",
  "state",
  "priority",
];
