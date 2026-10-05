/**
 * One decision point for which task chrome a conversation surface may render.
 *
 * A Quick Chat is a conversation, not a workflow task: the workflow stepper,
 * task-state actions, move/handoff actions, and pull-request panels have no
 * meaning there. Gating every Task-only control from one helper keeps the
 * surfaces from disagreeing and prevents an inert control from rendering.
 */
export type AppSurface = "task" | "quick-chat";

export type SurfaceCapabilities = {
  /** Workflow stepper and step transitions. */
  workflow: boolean;
  /** Task-state actions and the task actions menu (move, handoff, archive). */
  taskActions: boolean;
  /** Pull-request and review panels. */
  pullRequests: boolean;
};

export function surfaceCapabilities(surface: AppSurface): SurfaceCapabilities {
  const isTask = surface === "task";
  return {
    workflow: isTask,
    taskActions: isTask,
    pullRequests: isTask,
  };
}
