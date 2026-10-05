"use client";

import type { ComponentType, HTMLAttributes, ReactNode } from "react";
import type { Task } from "@/components/kanban-card";
import type { WorkflowSnapshotData } from "@/lib/state/slices/kanban/types";
import type { ViewGroup } from "@/lib/view-model/types";
import type { ViewContentProps } from "@/lib/kanban/view-registry";
import { SwimlaneSection } from "./swimlane-section";

export type ViewPropsForGroupLanes = Omit<
  ViewContentProps,
  "workflowId" | "steps" | "moveTargetSteps" | "tasks"
>;

export type WorkflowGroupLanesProps = {
  wf: { id: string; name: string };
  lanes: ViewGroup<Task>[];
  steps: WorkflowSnapshotData["steps"];
  moveTargetSteps: WorkflowSnapshotData["steps"];
  isCollapsed: boolean;
  onToggleCollapse: () => void;
  dragHandleProps?: HTMLAttributes<HTMLDivElement>;
  onToggleMultiSelect?: () => void;
  isMultiSelectMode?: boolean;
  ViewComponent: ComponentType<ViewContentProps>;
  viewProps: ViewPropsForGroupLanes;
  columnsMenu: ReactNode;
};

/**
 * Renders one workflow's board as group lanes (Repository Group, repository,
 * state, priority). The lane header label is the group's label; columns are the
 * workflow's own steps, so grouping is independent of the workflow axis.
 */
export function WorkflowGroupLanes({
  wf,
  lanes,
  steps,
  moveTargetSteps,
  isCollapsed,
  onToggleCollapse,
  dragHandleProps,
  onToggleMultiSelect,
  isMultiSelectMode,
  ViewComponent,
  viewProps,
  columnsMenu,
}: WorkflowGroupLanesProps) {
  return (
    <div className="space-y-3" data-testid="kanban-group-lanes">
      {lanes.map((lane, index) => (
        <SwimlaneSection
          key={lane.key}
          workflowId={`${wf.id}:${lane.key}`}
          workflowName={lane.label || wf.name}
          taskCount={lane.items.length}
          isCollapsed={isCollapsed}
          onToggleCollapse={onToggleCollapse}
          dragHandleProps={index === 0 ? dragHandleProps : undefined}
          onToggleMultiSelect={onToggleMultiSelect}
          isMultiSelectMode={isMultiSelectMode}
          columnsMenu={index === 0 ? columnsMenu : undefined}
        >
          <ViewComponent
            workflowId={wf.id}
            steps={steps}
            moveTargetSteps={moveTargetSteps}
            tasks={lane.items}
            {...viewProps}
          />
        </SwimlaneSection>
      ))}
    </div>
  );
}
