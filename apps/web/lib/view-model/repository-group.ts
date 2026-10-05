/**
 * Repository Group semantics (D7).
 *
 * A Repository Group is a `RepositorySet`: a named, ordered set of repositories.
 * A task matches a group when at least one of the task's repositories is a
 * member. Grouping places a task under the first group (in set order) that
 * contains the task's first repository; otherwise under Ungrouped.
 */
export type RepositoryGroup = {
  id: string;
  name: string;
  /** Ordered member repository ids. */
  repositoryIds: readonly string[];
};

export const UNGROUPED_REPOSITORY_GROUP_KEY = "__ungrouped__";

/** Whether a task with these repositories belongs to the group. */
export function taskMatchesRepositoryGroup(
  taskRepositoryIds: readonly string[],
  group: RepositoryGroup,
): boolean {
  return taskRepositoryIds.some((id) => group.repositoryIds.includes(id));
}

/** The ids of every group a task's repositories belong to, in set order. */
export function matchingRepositoryGroupIds(
  taskRepositoryIds: readonly string[],
  groups: readonly RepositoryGroup[],
): string[] {
  return groups
    .filter((group) => taskMatchesRepositoryGroup(taskRepositoryIds, group))
    .map((group) => group.id);
}

/**
 * The group key and label for a task: the first group in set order that holds
 * the task's first repository, else the explicit ungrouped bucket.
 */
export function repositoryGroupKeyAndLabel(
  taskRepositoryIds: readonly string[],
  groups: readonly RepositoryGroup[],
  ungroupedLabel: string,
): { key: string; label: string } {
  const first = taskRepositoryIds[0];
  if (first) {
    for (const group of groups) {
      if (group.repositoryIds.includes(first)) return { key: group.id, label: group.name };
    }
  }
  return { key: UNGROUPED_REPOSITORY_GROUP_KEY, label: ungroupedLabel };
}
