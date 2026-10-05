import type { RepositorySet } from "@/lib/types/http";
import type { RepositoryGroup } from "./repository-group";

/**
 * Project the workspace's persisted `RepositorySet[]` into the ordered
 * `RepositoryGroup` view the shared model consumes. Membership keeps the set's
 * stored order so grouping is deterministic.
 */
export function toRepositoryGroups(sets: readonly RepositorySet[] | undefined): RepositoryGroup[] {
  if (!sets || sets.length === 0) return [];
  return sets.map((set) => ({
    id: set.id,
    name: set.name,
    repositoryIds: (set.repositories ?? [])
      .slice()
      .sort((a, b) => a.position - b.position)
      .map((item) => String(item.repository_id)),
  }));
}
