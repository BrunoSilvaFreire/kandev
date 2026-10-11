import type { GroupedViewItems, ViewGroup, ViewGroupKey, ViewGroupNode } from "./types";

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

/** One level of a nested grouping: its key and how an item maps to a group. */
export type ViewGroupLevel<Item> = {
  groupKey: ViewGroupKey;
  extract: ViewGroupExtractor<Item>;
};

/**
 * Encodes a node's identity from the level keys leading to it. JSON keeps the
 * encoding unambiguous for any key text, and labels are never part of it, so
 * the identity survives a rename and a filter change.
 */
export function encodeGroupPath(keys: readonly string[]): string {
  return JSON.stringify(keys);
}

function groupLevel<Item>(
  items: Item[],
  levels: readonly ViewGroupLevel<Item>[],
  depth: number,
  parentKeys: readonly string[],
): ViewGroupNode<Item>[] {
  const level = levels[depth];
  if (!level) return [];
  return applyViewGroup(items, level.groupKey, level.extract).groups.map((group) => {
    const keys = [...parentKeys, group.key];
    return {
      key: group.key,
      label: group.label,
      groupKey: level.groupKey,
      path: encodeGroupPath(keys),
      items: group.items,
      children: groupLevel(group.items, levels, depth + 1, keys),
    };
  });
}

/**
 * Groups items by an ordered list of levels into a tree. The first level forms
 * the outer groups and each later level subdivides the group above it. Order at
 * every level is first-seen, so a stable item order gives a stable tree.
 */
export function applyViewGroupPath<Item>(
  items: Item[],
  levels: readonly ViewGroupLevel<Item>[],
): ViewGroupNode<Item>[] {
  return groupLevel(items, levels, 0, []);
}

/**
 * Normalizes a stored group list for a view: unsupported keys and repeats are
 * dropped, `none` ends the list, and an empty result is `["none"]`.
 */
export function normalizeGroupOrder(
  stored: readonly string[] | undefined,
  supported: readonly ViewGroupKey[],
): ViewGroupKey[] {
  const order: ViewGroupKey[] = [];
  for (const key of stored ?? []) {
    if (key === "none") break;
    const typed = key as ViewGroupKey;
    if (supported.includes(typed) && !order.includes(typed)) order.push(typed);
  }
  return order.length > 0 ? order : ["none"];
}
