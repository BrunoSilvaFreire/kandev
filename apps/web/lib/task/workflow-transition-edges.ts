import type { WorkflowStepperStep } from "@/components/task/workflow-step-disclosure";

/** One currently-possible directed transition edge for the expanded header. */
export type WorkflowTransitionEdge = {
  fromStepId: string;
  toStepId: string;
  /** A current-step manual destination, rendered dashed. */
  manual: boolean;
};

type StepAction = { type: string; config?: Record<string, unknown> };

const MOVE_TYPES = new Set(["move_to_next", "move_to_previous", "move_to_step"]);

const EVENT_KEYS = [
  "on_enter",
  "on_turn_start",
  "on_turn_complete",
  "on_exit",
  "on_comment",
  "on_blocker_resolved",
  "on_children_completed",
  "on_approval_resolved",
  "on_heartbeat",
  "on_budget_alert",
  "on_agent_error",
] as const;

/** Resolve a configured move action to its destination step, against the
 *  ordered step list (relative next/previous) or a step id/position target. */
function resolveTargetAction(
  action: StepAction,
  index: number,
  sorted: WorkflowStepperStep[],
): string | null {
  if (action.type === "move_to_next") return sorted[index + 1]?.id ?? null;
  if (action.type === "move_to_previous") return sorted[index - 1]?.id ?? null;
  if (action.type !== "move_to_step") return null;
  const config = action.config ?? {};
  if (typeof config.step_id === "string" && config.step_id) return config.step_id;
  if (typeof config.step_position === "number") {
    return sorted.find((step) => step.position === config.step_position)?.id ?? null;
  }
  return null;
}

/**
 * Derive each currently-possible directed transition edge exactly once.
 * Relative next/previous resolve against the current ordered positions,
 * duplicate configured actions collapse onto one pair, and manual
 * destinations are added only from the current step. Null-source initial
 * entry and removed edges never appear.
 */
export function deriveWorkflowTransitionEdges(
  steps: WorkflowStepperStep[],
  currentStepId: string | null,
): WorkflowTransitionEdge[] {
  const sorted = [...steps].sort((left, right) => left.position - right.position);
  const edges = new Map<string, WorkflowTransitionEdge>();
  const add = (from: string, to: string | null, manual: boolean) => {
    if (!from || !to || from === to) return;
    const key = `${from}->${to}`;
    const existing = edges.get(key);
    if (existing) {
      if (!manual) existing.manual = false;
      return;
    }
    edges.set(key, { fromStepId: from, toStepId: to, manual });
  };

  sorted.forEach((step, index) => {
    for (const key of EVENT_KEYS) {
      const actions = (step.events?.[key] ?? []) as StepAction[];
      for (const action of actions) {
        if (MOVE_TYPES.has(action.type)) {
          add(step.id, resolveTargetAction(action, index, sorted), false);
        }
      }
    }
    if (step.id === currentStepId && step.allow_manual_move) {
      for (const other of sorted) add(step.id, other.id, true);
    }
  });

  return [...edges.values()].sort(
    (left, right) =>
      left.fromStepId.localeCompare(right.fromStepId) ||
      left.toStepId.localeCompare(right.toStepId),
  );
}
