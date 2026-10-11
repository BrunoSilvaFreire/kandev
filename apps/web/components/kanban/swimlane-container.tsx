"use client";

import {
  memo,
  type ComponentType,
  type HTMLAttributes,
  useCallback,
  useEffect,
  useMemo,
} from "react";
import { DndContext, closestCenter } from "@dnd-kit/core";
import { SortableContext, verticalListSortingStrategy, useSortable } from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { useAppStore } from "@/components/state-provider";
import { useSwimlaneCollapse } from "@/hooks/domains/kanban/use-swimlane-collapse";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { selectVisibleWorkflows } from "@/lib/kanban/workflow-swimlanes";
import { SwimlaneSection } from "./swimlane-section";
import { ColumnsMenu } from "./columns-menu";
import { useKanbanDisplaySettings } from "@/hooks/use-kanban-display-settings";
import {
  useWorkflowGroupLanes,
  type KanbanGroupScope,
} from "@/hooks/domains/kanban/use-workflow-group-lanes";
import type { ViewGroupKey, ViewGroupNode } from "@/lib/view-model/types";
import {
  useBoardGrouping,
  useVcsSearchIndex,
  useVisibleWorkflowSteps,
  useWorkflowReorder,
} from "./swimlane-container-hooks";
import { KanbanGlobalGroupHeader } from "./kanban-global-group-header";
import { WorkflowGroupLanes } from "./workflow-group-lanes";
import {
  getEffectiveView,
  type MobileWorkflowNavigation,
  type ViewContentProps,
} from "@/lib/kanban/view-registry";
import type { Task } from "@/components/kanban-card";
import type { MoveTaskError } from "@/hooks/use-drag-and-drop";
import type { WorkflowSnapshotData } from "@/lib/state/slices/kanban/types";
import { t } from "@/lib/i18n";
import {
  useStableWorkflowList,
  useSwimlaneRenderData,
  useWorkflowSwimlaneData,
} from "@/hooks/domains/kanban/use-swimlane-render-data";

export type SwimlaneContainerProps = {
  viewMode: string;
  workflowFilter: string | null;
  onPreviewTask: (task: Task) => void;
  onOpenTask: (task: Task) => void;
  onEditTask: (task: Task) => void;
  onDeleteTask: (task: Task) => void;
  onArchiveTask?: (task: Task) => void;
  onMoveError?: (error: MoveTaskError) => void;
  deletingTaskId?: string | null;
  archivingTaskId?: string | null;
  showMaximizeButton?: boolean;
  searchQuery?: string;
  selectedRepositoryIds?: string[];
  /** Predicate combining every active plugin `registerTaskFilter` selection (AND semantics). */
  matchesPluginTaskFilters?: (taskId: string) => boolean;
  selectedIds?: Set<string>;
  onToggleSelect?: (taskId: string) => void;
  onSelectRange?: (taskId: string, orderedIds: string[]) => void;
  isMultiSelectMode?: boolean;
  onToggleMultiSelect?: () => void;
  onWorkflowChange?: (workflowId: string | null) => void;
};

type EmptyMessageOptions = {
  isLoading: boolean;
  snapshots: Record<string, unknown>;
  orderedWorkflows: { id: string; name: string }[];
  visibleWorkflows: { id: string; name: string }[];
  showEmptyBoard: boolean;
};

function getEmptyMessage({
  isLoading,
  snapshots,
  orderedWorkflows,
  visibleWorkflows,
  showEmptyBoard,
}: EmptyMessageOptions): string | null {
  if (isLoading && Object.keys(snapshots).length === 0) return t("common:loading");
  if (orderedWorkflows.length === 0) return t("kanban:noWorkflowsAvailableYet");
  if (visibleWorkflows.length === 0 && !showEmptyBoard) return t("kanban:noTasksYet");
  return null;
}

function renderEmptyState(emptyMessage: string) {
  return (
    <div className="flex-1 min-h-0 px-4 pt-3 pb-4">
      <div className="h-full rounded-lg border border-dashed border-border/60 flex items-center justify-center text-sm text-muted-foreground">
        {emptyMessage}
      </div>
    </div>
  );
}

const EMPTY_SELECTED_REPOSITORY_IDS: string[] = [];
const EMPTY_WORKFLOW_STEPS: WorkflowSnapshotData["steps"] = [];

type WorkflowItemProps = {
  wf: { id: string; name: string };
  repoFilter: Set<string>;
  searchQuery: string;
  vcsSearchTextByTaskId?: Record<string, string>;
  matchesPluginTaskFilters?: (taskId: string) => boolean;
  ViewComponent: ComponentType<ViewContentProps>;
  hideHeader: boolean;
  isSortable: boolean;
  isCollapsed: boolean;
  toggleCollapse: (workflowId: string) => void;
  onPreviewTask: (task: Task) => void;
  onOpenTask: (task: Task) => void;
  onEditTask: (task: Task) => void;
  onDeleteTask: (task: Task) => void;
  onArchiveTask?: (task: Task) => void;
  onMoveError?: (error: MoveTaskError) => void;
  deletingTaskId?: string | null;
  archivingTaskId?: string | null;
  showMaximizeButton?: boolean;
  selectedIds?: Set<string>;
  onToggleSelect?: (taskId: string) => void;
  onSelectRange?: (taskId: string, orderedIds: string[]) => void;
  isMultiSelectMode?: boolean;
  onToggleMultiSelect?: () => void;
  fillHeight?: boolean;
  compactHeight?: boolean;
  mobileWorkflowNavigation?: MobileWorkflowNavigation;
  onToggleStepVisibility: (workflowId: string, stepId: string) => void;
  onToggleAutoHideEmpty: (workflowId: string) => void;
  /** Group keys that subdivide this workflow's board into lanes. */
  laneKeys: readonly ViewGroupKey[];
  /** The outer group this board is rendered under, when grouping is global. */
  scope?: KanbanGroupScope;
};

const SortableWorkflowItem = memo(function SortableWorkflowItem({
  wf,
  hideHeader,
  isSortable,
  fillHeight,
  toggleCollapse,
  ...rest
}: WorkflowItemProps) {
  // Under an outer group the same workflow renders once per group, so its
  // sortable identity includes the group.
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id: rest.scope ? `${rest.scope.key}:${wf.id}` : wf.id,
    disabled: !isSortable,
  });
  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
    opacity: isDragging ? 0.5 : 1,
  };
  const dragHandleProps = useMemo(
    () => (isSortable && !hideHeader ? { ...attributes, ...listeners } : undefined),
    [attributes, hideHeader, isSortable, listeners],
  );
  const onToggleCollapse = useCallback(() => toggleCollapse(wf.id), [toggleCollapse, wf.id]);
  return (
    <div
      ref={setNodeRef}
      style={style}
      className={fillHeight ? "h-full min-h-0 flex-1" : undefined}
    >
      <WorkflowItemContent
        wf={wf}
        hideHeader={hideHeader}
        fillHeight={fillHeight}
        dragHandleProps={dragHandleProps}
        onToggleCollapse={onToggleCollapse}
        {...rest}
      />
    </div>
  );
});

type WorkflowItemContentProps = Omit<WorkflowItemProps, "isSortable" | "toggleCollapse"> & {
  dragHandleProps?: HTMLAttributes<HTMLDivElement>;
  onToggleCollapse: () => void;
};

const WorkflowItemContent = memo(function WorkflowItemContent({
  wf,
  repoFilter,
  searchQuery,
  vcsSearchTextByTaskId,
  matchesPluginTaskFilters,
  ViewComponent,
  hideHeader,
  isCollapsed,
  onToggleCollapse,
  dragHandleProps,
  onToggleMultiSelect,
  fillHeight,
  onToggleStepVisibility,
  onToggleAutoHideEmpty,
  laneKeys,
  scope,
  ...viewProps
}: WorkflowItemContentProps) {
  const data = useWorkflowSwimlaneData(
    wf.id,
    repoFilter,
    searchQuery,
    matchesPluginTaskFilters,
    vcsSearchTextByTaskId,
  );
  const { snapshot, occupancyTasks, hiddenStepIds, hiddenSet, autoHideEmpty } = data;
  const { tasks, lanes } = useWorkflowGroupLanes(wf, data.tasks, laneKeys, scope);
  const snapshotSteps = snapshot?.steps ?? EMPTY_WORKFLOW_STEPS;
  const { moveTargetSteps, steps } = useVisibleWorkflowSteps(
    snapshotSteps,
    occupancyTasks,
    autoHideEmpty,
    hiddenStepIds,
    hiddenSet,
  );
  const content = (
    <ViewComponent
      workflowId={wf.id}
      steps={steps}
      moveTargetSteps={moveTargetSteps}
      tasks={tasks}
      {...viewProps}
    />
  );

  if (!snapshot) return null;
  // A workflow with no task in this outer group is not part of the group.
  if (scope && tasks.length === 0) return null;

  const columnsMenu = (
    <ColumnsMenu
      workflowId={wf.id}
      workflowName={wf.name}
      steps={snapshotSteps}
      hiddenStepIds={hiddenStepIds}
      onToggle={onToggleStepVisibility}
      autoHideEmpty={autoHideEmpty}
      onToggleAutoHide={onToggleAutoHideEmpty}
    />
  );

  if (lanes) {
    return (
      <WorkflowGroupLanes
        wf={wf}
        lanes={lanes}
        steps={steps}
        moveTargetSteps={moveTargetSteps}
        isCollapsed={isCollapsed}
        onToggleCollapse={onToggleCollapse}
        dragHandleProps={dragHandleProps}
        onToggleMultiSelect={onToggleMultiSelect}
        isMultiSelectMode={viewProps.isMultiSelectMode}
        ViewComponent={ViewComponent}
        viewProps={viewProps}
        columnsMenu={columnsMenu}
      />
    );
  }

  if (hideHeader) {
    return <div className={fillHeight ? "h-full min-h-0" : undefined}>{content}</div>;
  }

  return (
    <SwimlaneSection
      workflowId={wf.id}
      workflowName={wf.name}
      taskCount={tasks.length}
      isCollapsed={isCollapsed}
      onToggleCollapse={onToggleCollapse}
      dragHandleProps={dragHandleProps}
      onToggleMultiSelect={onToggleMultiSelect}
      isMultiSelectMode={viewProps.isMultiSelectMode}
      fillHeight={fillHeight}
      columnsMenu={columnsMenu}
    >
      {content}
    </SwimlaneSection>
  );
});

function getRenderedWorkflows(
  isMobileKanban: boolean,
  focusedWorkflowId: string | null,
  visibleWorkflows: { id: string; name: string }[],
) {
  if (!isMobileKanban || !focusedWorkflowId) return visibleWorkflows;
  return visibleWorkflows.filter((workflow) => workflow.id === focusedWorkflowId);
}

function getContainerClass(isMobileKanban: boolean, isMobile: boolean): string {
  if (isMobileKanban) return "flex flex-1 min-h-0 flex-col overflow-hidden pb-4";
  return `flex-1 min-h-0 space-y-3 overflow-y-auto pt-3 pb-4${isMobile ? "" : " px-4"}`;
}

function shouldHideHeaders(
  isMobile: boolean,
  isMobileKanban: boolean,
  workflowFilter: string | null,
  workflowCount: number,
): boolean {
  if (!isMobile) return false;
  return isMobileKanban || workflowFilter !== null || workflowCount === 1;
}

type WorkflowItemsProps = {
  workflows: { id: string; name: string }[];
  repoFilter: Set<string>;
  searchQuery: string;
  vcsSearchTextByTaskId?: Record<string, string>;
  matchesPluginTaskFilters?: (taskId: string) => boolean;
  ViewComponent: ComponentType<ViewContentProps>;
  hideHeaders: boolean;
  fillHeight: boolean;
  compactHeight: boolean;
  isMobileKanban: boolean;
  onToggleStepVisibility: (workflowId: string, stepId: string) => void;
  onToggleAutoHideEmpty: (workflowId: string) => void;
  canSortWorkflows: boolean;
  isCollapsed: (workflowId: string) => boolean;
  toggleCollapse: (workflowId: string) => void;
  containerProps: SwimlaneContainerProps;
  mobileWorkflowNavigation?: MobileWorkflowNavigation;
  laneKeys: readonly ViewGroupKey[];
  scope?: KanbanGroupScope;
};

function WorkflowItems({
  workflows,
  repoFilter,
  searchQuery,
  vcsSearchTextByTaskId,
  matchesPluginTaskFilters,
  ViewComponent,
  hideHeaders,
  fillHeight,
  compactHeight,
  isMobileKanban,
  onToggleStepVisibility,
  onToggleAutoHideEmpty,
  canSortWorkflows,
  isCollapsed,
  toggleCollapse,
  containerProps,
  mobileWorkflowNavigation,
  laneKeys,
  scope,
}: WorkflowItemsProps) {
  return workflows.map((workflow, index) => {
    const collapsed = isCollapsed(workflow.id);
    return (
      <SortableWorkflowItem
        key={isMobileKanban ? "mobile-active-workflow" : workflow.id}
        wf={workflow}
        laneKeys={laneKeys}
        scope={scope}
        repoFilter={repoFilter}
        searchQuery={searchQuery}
        vcsSearchTextByTaskId={vcsSearchTextByTaskId}
        matchesPluginTaskFilters={matchesPluginTaskFilters}
        ViewComponent={ViewComponent}
        hideHeader={hideHeaders}
        fillHeight={fillHeight && !collapsed}
        compactHeight={compactHeight}
        isSortable={canSortWorkflows && !isMobileKanban}
        isCollapsed={collapsed}
        toggleCollapse={toggleCollapse}
        onPreviewTask={containerProps.onPreviewTask}
        onOpenTask={containerProps.onOpenTask}
        onEditTask={containerProps.onEditTask}
        onDeleteTask={containerProps.onDeleteTask}
        onArchiveTask={containerProps.onArchiveTask}
        onMoveError={containerProps.onMoveError}
        deletingTaskId={containerProps.deletingTaskId}
        archivingTaskId={containerProps.archivingTaskId}
        showMaximizeButton={containerProps.showMaximizeButton}
        selectedIds={containerProps.selectedIds}
        onToggleSelect={containerProps.onToggleSelect}
        onSelectRange={containerProps.onSelectRange}
        isMultiSelectMode={containerProps.isMultiSelectMode}
        onToggleMultiSelect={index === 0 ? containerProps.onToggleMultiSelect : undefined}
        mobileWorkflowNavigation={mobileWorkflowNavigation}
        onToggleStepVisibility={onToggleStepVisibility}
        onToggleAutoHideEmpty={onToggleAutoHideEmpty}
      />
    );
  });
}

type GroupedWorkflowItemsProps = Omit<WorkflowItemsProps, "scope"> & {
  outerKey: ViewGroupKey | null;
  outerGroups: ViewGroupNode<Task>[];
  compactWhenGrouped: boolean;
};

/**
 * Renders the workflow boards, under one section per outer group when the
 * grouping is global. Inside a group each workflow keeps its header, appears
 * only if it has tasks there, and is not reorderable.
 */
function GroupedWorkflowItems({
  outerKey,
  outerGroups,
  compactWhenGrouped,
  ...items
}: GroupedWorkflowItemsProps) {
  if (!outerKey) return <WorkflowItems {...items} />;
  return outerGroups.map((group) => (
    <section
      key={group.path}
      className="space-y-3"
      data-testid="kanban-global-group"
      data-group-key={group.key}
    >
      <KanbanGlobalGroupHeader label={group.label} count={group.items.length} />
      <WorkflowItems
        {...items}
        hideHeaders={false}
        fillHeight={false}
        compactHeight={compactWhenGrouped}
        canSortWorkflows={false}
        scope={{ groupKey: outerKey, key: group.key }}
      />
    </section>
  ));
}

/**
 * Publishes which workflow the phone board is showing to the store so the
 * mobile menu drawer's Columns control targets that same workflow instead of
 * re-deriving focus from filters.
 */
function usePublishMobileFocus(focusedWorkflowId: string | null) {
  const setMobileKanbanFocusedWorkflow = useAppStore(
    (state) => state.setMobileKanbanFocusedWorkflow,
  );
  useEffect(() => {
    setMobileKanbanFocusedWorkflow(focusedWorkflowId);
  }, [focusedWorkflowId, setMobileKanbanFocusedWorkflow]);
}

type RenderedWorkflowLayoutOptions = {
  workflowFilter: string | null;
  orderedWorkflows: { id: string; name: string }[];
  getFilteredTasks: (workflowId: string) => Task[];
  hasLiveHiddenSteps: (workflowId: string) => boolean;
  isMobileKanban: boolean;
  workflowOptions: MobileWorkflowNavigation["workflows"];
  onWorkflowChange: SwimlaneContainerProps["onWorkflowChange"];
};

function useRenderedWorkflowLayout({
  workflowFilter,
  orderedWorkflows,
  getFilteredTasks,
  hasLiveHiddenSteps,
  isMobileKanban,
  workflowOptions,
  onWorkflowChange,
}: RenderedWorkflowLayoutOptions) {
  const visibleWorkflows = useStableWorkflowList(
    selectVisibleWorkflows({
      workflowFilter,
      orderedWorkflows,
      hasTasks: (workflowId) => getFilteredTasks(workflowId).length > 0,
      hasLiveHiddenSteps,
      showEmptyBoard: isMobileKanban,
    }),
  );
  const focusedWorkflowId = visibleWorkflows[0]?.id ?? null;
  usePublishMobileFocus(isMobileKanban ? focusedWorkflowId : null);
  const renderedWorkflows = useStableWorkflowList(
    getRenderedWorkflows(isMobileKanban, focusedWorkflowId, visibleWorkflows),
  );
  const sortableWorkflowIds = useMemo(
    () => renderedWorkflows.map((workflow) => workflow.id),
    [renderedWorkflows],
  );
  const mobileWorkflowNavigation = useMobileWorkflowNavigation(
    isMobileKanban,
    focusedWorkflowId,
    workflowOptions,
    onWorkflowChange,
  );
  return { visibleWorkflows, renderedWorkflows, sortableWorkflowIds, mobileWorkflowNavigation };
}

function useMobileWorkflowNavigation(
  isMobileKanban: boolean,
  focusedWorkflowId: string | null,
  workflowOptions: MobileWorkflowNavigation["workflows"],
  onWorkflowChange: SwimlaneContainerProps["onWorkflowChange"],
) {
  return useMemo(
    () =>
      isMobileKanban && focusedWorkflowId && onWorkflowChange
        ? { activeWorkflowId: focusedWorkflowId, workflows: workflowOptions, onWorkflowChange }
        : undefined,
    [focusedWorkflowId, isMobileKanban, onWorkflowChange, workflowOptions],
  );
}

export function SwimlaneContainer(containerProps: SwimlaneContainerProps) {
  const {
    viewMode,
    workflowFilter,
    searchQuery,
    selectedRepositoryIds = EMPTY_SELECTED_REPOSITORY_IDS,
  } = containerProps;
  const { isMobile } = useResponsiveBreakpoint();
  const { groups, onToggleStepVisibility, onToggleAutoHideEmpty } = useKanbanDisplaySettings();
  const { isCollapsed, toggleCollapse } = useSwimlaneCollapse();
  const vcsSearchTextByTaskId = useVcsSearchIndex();
  const {
    snapshots,
    isLoading,
    orderedWorkflows,
    workflowOptions,
    getFilteredTasks,
    hasLiveHiddenSteps,
    repoFilter,
  } = useSwimlaneRenderData(
    workflowFilter,
    selectedRepositoryIds,
    searchQuery ?? "",
    containerProps.matchesPluginTaskFilters,
    vcsSearchTextByTaskId,
  );
  const {
    sensors: workflowSensors,
    canSort: canSortWorkflows,
    handleDragEnd: handleWorkflowDragEnd,
  } = useWorkflowReorder(orderedWorkflows, workflowFilter);

  const view = getEffectiveView(viewMode, isMobile);
  const isMobileKanban = isMobile && view.id === "kanban";
  const { visibleWorkflows, renderedWorkflows, sortableWorkflowIds, mobileWorkflowNavigation } =
    useRenderedWorkflowLayout({
      workflowFilter,
      orderedWorkflows,
      getFilteredTasks,
      hasLiveHiddenSteps,
      isMobileKanban,
      workflowOptions,
      onWorkflowChange: containerProps.onWorkflowChange,
    });

  const { outerKey, laneKeys, outerGroups } = useBoardGrouping({
    groups,
    isMobileKanban,
    renderedWorkflows,
    getFilteredTasks,
  });

  const emptyMessage = getEmptyMessage({
    isLoading,
    snapshots,
    orderedWorkflows,
    visibleWorkflows,
    showEmptyBoard: isMobileKanban,
  });
  if (emptyMessage) return renderEmptyState(emptyMessage);

  const hideHeaders = shouldHideHeaders(
    isMobile,
    isMobileKanban,
    workflowFilter,
    orderedWorkflows.length,
  );
  const containerClass = getContainerClass(isMobileKanban, isMobile);
  return (
    <DndContext
      sensors={workflowSensors}
      collisionDetection={closestCenter}
      onDragEnd={handleWorkflowDragEnd}
    >
      <SortableContext items={sortableWorkflowIds} strategy={verticalListSortingStrategy}>
        <div className={containerClass} data-testid="swimlane-container">
          <GroupedWorkflowItems
            outerKey={outerKey}
            outerGroups={outerGroups}
            laneKeys={laneKeys}
            compactWhenGrouped={view.id === "kanban" && !isMobile}
            workflows={renderedWorkflows}
            repoFilter={repoFilter}
            searchQuery={searchQuery ?? ""}
            vcsSearchTextByTaskId={vcsSearchTextByTaskId}
            matchesPluginTaskFilters={containerProps.matchesPluginTaskFilters}
            ViewComponent={view.component}
            hideHeaders={hideHeaders}
            onToggleStepVisibility={onToggleStepVisibility}
            onToggleAutoHideEmpty={onToggleAutoHideEmpty}
            fillHeight={view.id === "kanban" && renderedWorkflows.length === 1}
            compactHeight={view.id === "kanban" && !isMobile && renderedWorkflows.length > 1}
            isMobileKanban={isMobileKanban}
            canSortWorkflows={canSortWorkflows}
            isCollapsed={isCollapsed}
            toggleCollapse={toggleCollapse}
            containerProps={containerProps}
            mobileWorkflowNavigation={mobileWorkflowNavigation}
          />
        </div>
      </SortableContext>
    </DndContext>
  );
}
