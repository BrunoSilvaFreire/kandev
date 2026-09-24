import {
  normalizeWorkflowProfileSessionEndPolicy,
  normalizeWorkflowProfileSessionStartPolicy,
  type WorkflowStep,
} from "@/lib/types/http";

function sessionPolicyPayload(step: WorkflowStep): Partial<WorkflowStep> {
  return {
    profile_session_start_policy: normalizeWorkflowProfileSessionStartPolicy(
      step.profile_session_start_policy,
    ),
    profile_session_end_policy: normalizeWorkflowProfileSessionEndPolicy(
      step.profile_session_end_policy,
    ),
  };
}

function numericPolicyPayload(step: WorkflowStep): Partial<WorkflowStep> {
  return {
    auto_archive_after_hours: step.auto_archive_after_hours ?? 0,
    wip_limit: step.wip_limit ?? 0,
  };
}

function stepUpdatePayload(step: WorkflowStep): Partial<WorkflowStep> {
  return {
    name: step.name,
    position: step.position,
    color: step.color,
    stage_type: step.stage_type ?? "custom",
    prompt: step.prompt ?? "",
    events: step.events ?? {},
    allow_manual_move: step.allow_manual_move ?? true,
    is_start_step: step.is_start_step ?? false,
    show_in_command_panel: step.show_in_command_panel ?? false,
    agent_profile_id: step.agent_profile_id ?? "",
    allowed_tags: step.allowed_tags ?? [],
    ...sessionPolicyPayload(step),
    ...numericPolicyPayload(step),
    session_target: step.session_target ?? null,
    complete_task_on_enter: step.complete_task_on_enter ?? false,
    auto_advance_requires_signal: step.auto_advance_requires_signal ?? false,
    cancel_triggers_turn_complete: step.cancel_triggers_turn_complete ?? false,
    pull_from_step_id: step.pull_from_step_id ?? "",
  };
}

export function areStepDraftsEqual(left: WorkflowStep[], right: WorkflowStep[]): boolean;
export function areStepDraftsEqual(left: WorkflowStep, right: WorkflowStep): boolean;
export function areStepDraftsEqual(
  left: WorkflowStep[] | WorkflowStep,
  right: WorkflowStep[] | WorkflowStep,
): boolean {
  if (Array.isArray(left) && Array.isArray(right)) {
    if (left.length !== right.length) return false;
    return left.every((step, index) => areStepDraftsEqual(step, right[index]));
  }
  if (Array.isArray(left) || Array.isArray(right)) return false;
  return JSON.stringify(stepUpdatePayload(left)) === JSON.stringify(stepUpdatePayload(right));
}

export { stepUpdatePayload };
