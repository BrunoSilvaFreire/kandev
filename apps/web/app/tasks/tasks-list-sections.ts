import { t } from "@/lib/i18n";
import { formatTaskStateLabel } from "@/lib/ui/state-labels";
import { primaryTaskRepository, type Task } from "@/lib/types/http";
import { TASK_STATE_ORDER } from "@/lib/tasks/tasks-list-options";
import { repositoryGroupKeyAndLabel } from "@/lib/view-model/repository-group";
import type { RepositoryGroup } from "@/lib/view-model/repository-group";
import type { TaskListFacetValue } from "@/lib/plugins/types";

export type TaskTreeNode = {
  task: Task;
  children: TaskTreeNode[];
  level: number;
};

export type TaskListSection = {
  key: string;
  title: string | null;
  color?: string;
  nodes: TaskTreeNode[];
};

const UNGROUPED_FACET_SECTION_KEY = "facet:host:ungrouped";

function facetValueSectionKey(value: string): string {
  return `facet:value:${value}`;
}

export function buildTaskSections(
  tasks: Task[],
  {
    groupBy,
    workflowMap,
    repoMap,
    facetValues,
    repositoryGroups = [],
  }: {
    groupBy: string;
    workflowMap: Map<string, string>;
    repoMap: Map<string, string>;
    facetValues: Record<string, readonly TaskListFacetValue[]>;
    repositoryGroups?: readonly RepositoryGroup[];
  },
): TaskListSection[] {
  const roots = buildTaskTree(tasks);
  if (groupBy.startsWith("facet:")) {
    const grouped = new Map<string, { title: string; color?: string; tasks: Task[] }>();
    for (const task of tasks) {
      const values = facetValues[`${groupBy}:${task.id}`] ?? [];
      const entries = values.length ? values : [{ value: "untagged", label: t("tasks:ungrouped") }];
      for (const value of entries) {
        const key = values.length ? facetValueSectionKey(value.value) : UNGROUPED_FACET_SECTION_KEY;
        const section = grouped.get(key) ?? { title: value.label, color: value.color, tasks: [] };
        section.tasks.push(task);
        grouped.set(key, section);
      }
    }
    return Array.from(grouped.entries())
      .map(([key, section]) => ({
        key,
        title: section.title,
        color: section.color,
        nodes: buildTaskTree(section.tasks),
      }))
      .sort((a, b) =>
        (a.title ?? "").localeCompare(b.title ?? "", undefined, { sensitivity: "base" }),
      );
  }
  if (groupBy === "none") {
    return [{ key: "all", title: null, nodes: roots }];
  }

  const sections = new Map<string, TaskListSection>();
  for (const node of roots) {
    const { key, title } = groupForTask(node.task, groupBy, workflowMap, repoMap, repositoryGroups);
    const section = sections.get(key) ?? { key, title, nodes: [] };
    section.nodes.push(node);
    sections.set(key, section);
  }

  return Array.from(sections.values()).sort((a, b) => compareSection(a, b, groupBy));
}

function buildTaskTree(tasks: Task[]): TaskTreeNode[] {
  const childrenByParent = new Map<string, Task[]>();
  const taskIds = new Set(tasks.map((task) => task.id));
  const roots: Task[] = [];

  for (const task of tasks) {
    if (task.parent_id && taskIds.has(task.parent_id)) {
      const siblings = childrenByParent.get(task.parent_id) ?? [];
      siblings.push(task);
      childrenByParent.set(task.parent_id, siblings);
    } else {
      roots.push(task);
    }
  }

  const visited = new Set<string>();

  const buildNode = (task: Task, level: number): TaskTreeNode | null => {
    if (visited.has(task.id)) return null;
    visited.add(task.id);
    return {
      task,
      level,
      children: (childrenByParent.get(task.id) ?? [])
        .map((child) => buildNode(child, level + 1))
        .filter((node): node is TaskTreeNode => node !== null),
    };
  };

  const nodes = roots
    .map((task) => buildNode(task, 0))
    .filter((node): node is TaskTreeNode => node !== null);
  for (const task of tasks) {
    const node = buildNode(task, 0);
    if (node) nodes.push(node);
  }

  return nodes;
}

function groupForTask(
  task: Task,
  groupBy: string,
  workflowMap: Map<string, string>,
  repoMap: Map<string, string>,
  repositoryGroups: readonly RepositoryGroup[] = [],
) {
  if (groupBy === "workflow") {
    const title = workflowMap.get(task.workflow_id);
    if (!title) return { key: "workflow:none", title: t("tasks:noWorkflow") };
    return { key: `workflow:${task.workflow_id || "none"}`, title };
  }
  if (groupBy === "repositoryGroup") {
    const ids = (task.repositories ?? []).map((repo) => String(repo.repository_id));
    const { key, label } = repositoryGroupKeyAndLabel(
      ids,
      repositoryGroups,
      t("sidebar:groupUnassigned"),
    );
    return { key: `repositoryGroup:${key}`, title: label };
  }
  if (groupBy === "repository") {
    const primaryRepo = primaryTaskRepository(task.repositories);
    if (!primaryRepo) return { key: "repository:none", title: t("tasks:noRepository") };
    const repoId = primaryRepo?.repository_id ?? "none";
    const title = repoMap.get(repoId);
    if (!title) return { key: "repository:none", title: t("tasks:noRepository") };
    return { key: `repository:${repoId}`, title };
  }
  const title = formatTaskStateLabel(task.state);
  return { key: `state:${task.state}`, title };
}

function compareSection(a: TaskListSection, b: TaskListSection, groupBy: string): number {
  if (groupBy === "state") {
    const aIndex = TASK_STATE_ORDER.indexOf(a.key.replace("state:", "") as Task["state"]);
    const bIndex = TASK_STATE_ORDER.indexOf(b.key.replace("state:", "") as Task["state"]);
    return (
      (aIndex === -1 ? Number.MAX_SAFE_INTEGER : aIndex) -
      (bIndex === -1 ? Number.MAX_SAFE_INTEGER : bIndex)
    );
  }
  return (a.title ?? "").localeCompare(b.title ?? "", undefined, { sensitivity: "base" });
}

export function flattenTaskTree(nodes: TaskTreeNode[]): TaskTreeNode[] {
  return nodes.flatMap((node) => [node, ...flattenTaskTree(node.children)]);
}
