/**
 * The shared envelope the backend returns for a declined or failed workflow
 * transition (REQ-TASKS-TRANSITION-DIAGNOSTICS-001). Every move surface parses
 * it here; older backends send no envelope and callers keep their fallback.
 */
import { ApiError } from "@/lib/api/client";

export type TransitionFailureReason = {
  rule: string;
  criterion_id?: string;
  step_id?: string;
  description?: string;
  observed?: string;
  required?: string;
  action: string;
  cta?: { kind: string; target?: string };
};

export type TransitionFailure = {
  code: string;
  message: string;
  actionability: "user" | "system";
  reasons: TransitionFailureReason[];
  remediation?: string;
  incident_id?: string;
};

/** A transition that failed after its move or completion signal was accepted. */
export type TransitionFailureRecord = {
  task_id: string;
  attempt_id: string;
  source: "deferred_move" | "auto_advance";
  created_at: string;
  failure: TransitionFailure;
};

// Catalog keys, not copy: resolved with t() at render.
const HEADLINE_KEYS: Record<string, string> = {
  task_move_active_session: "task:taskMoveErrorActiveSession",
  task_move_archived: "task:taskMoveErrorArchived",
  task_move_different_workspace: "task:taskMoveErrorDifferentWorkspace",
  task_move_workflow_step: "task:taskMoveErrorWorkflowStep",
  task_move_wip_limit: "task:taskMoveErrorWipLimit",
  task_completion_gate_blocked: "task:taskMoveErrorGateBlocked",
  task_completion_criteria_conflict: "task:taskMoveErrorCriteriaConflict",
  task_completion_human_confirmation_required: "task:taskMoveErrorHumanConfirmation",
  task_move_pending: "task:taskMoveErrorPending",
  task_move_stale: "task:taskMoveErrorStale",
  workflow_change_conflict: "task:taskMoveErrorStale",
  task_move_invalid: "task:taskMoveErrorInvalid",
  task_transition_unknown: "task:taskMoveErrorTransitionUnknown",
  task_move_not_found: "task:taskMoveErrorNotFound",
  task_transition_internal_failure: "task:taskMoveErrorInternal",
};

const REASON_KEYS: Record<string, string> = {
  "completion_gate.plan_stale": "task:transitionReasonPlanStale",
  "completion_gate.criterion_unverified": "task:transitionReasonCriterionUnverified",
  "completion_gate.evidence_stale": "task:transitionReasonEvidenceStale",
  "completion_gate.criteria_changed": "task:transitionReasonCriteriaChanged",
  "completion_gate.human_confirmation_required": "task:transitionReasonHumanConfirmation",
  "move.archived": "task:transitionReasonArchived",
  "move.active_session": "task:transitionReasonActiveSession",
  "move.different_workspace": "task:transitionReasonDifferentWorkspace",
  "move.step_not_in_workflow": "task:transitionReasonStepNotInWorkflow",
  "move.wip_limit": "task:transitionReasonWipLimit",
  "move.pending_move": "task:transitionReasonPendingMove",
  "move.task_changed": "task:transitionReasonTaskChanged",
  "move.entry_options": "task:transitionReasonEntryOptions",
  "move.entry_target_unavailable": "task:transitionReasonEntryTarget",
  "move.invalid_request": "task:transitionReasonInvalidRequest",
  "transition.unknown": "task:transitionReasonUnknownTransition",
  "move.target_not_found": "task:transitionReasonTargetNotFound",
  "system.internal": "task:transitionReasonInternal",
};

const GENERIC_REASON_KEY = "task:transitionReasonGeneric";

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function parseReason(value: unknown): TransitionFailureReason | null {
  if (!isRecord(value) || typeof value.rule !== "string") return null;
  const text = (key: string) =>
    typeof value[key] === "string" ? (value[key] as string) : undefined;
  const cta = isRecord(value.cta) && typeof value.cta.kind === "string" ? value.cta : null;
  return {
    rule: value.rule,
    criterion_id: text("criterion_id"),
    step_id: text("step_id"),
    description: text("description"),
    observed: text("observed"),
    required: text("required"),
    action: text("action") ?? "",
    cta: cta
      ? {
          kind: cta.kind as string,
          target: typeof cta.target === "string" ? cta.target : undefined,
        }
      : undefined,
  };
}

/** Parses an envelope from any transport; null when the value is not one. */
export function parseTransitionFailure(value: unknown): TransitionFailure | null {
  if (!isRecord(value) || typeof value.code !== "string" || !Array.isArray(value.reasons)) {
    return null;
  }
  return {
    code: value.code,
    message: typeof value.message === "string" ? value.message : "",
    actionability: value.actionability === "system" ? "system" : "user",
    reasons: value.reasons.flatMap((reason) => parseReason(reason) ?? []),
    remediation: typeof value.remediation === "string" ? value.remediation : undefined,
    incident_id: typeof value.incident_id === "string" ? value.incident_id : undefined,
  };
}

/** The envelope carried by a rejected move request, when the server sent one. */
export function getTransitionFailure(error: unknown): TransitionFailure | null {
  if (!(error instanceof ApiError) || !isRecord(error.body)) return null;
  return parseTransitionFailure(error.body.failure);
}

export function parseTransitionFailureRecord(value: unknown): TransitionFailureRecord | null {
  if (!isRecord(value) || typeof value.task_id !== "string") return null;
  const failure = parseTransitionFailure(value.failure);
  if (!failure) return null;
  return {
    task_id: value.task_id,
    attempt_id: typeof value.attempt_id === "string" ? value.attempt_id : "",
    source: value.source === "deferred_move" ? "deferred_move" : "auto_advance",
    created_at: typeof value.created_at === "string" ? value.created_at : "",
    failure,
  };
}

/** Catalog key for the failure's headline, or null for a code this build does not know. */
export function transitionFailureHeadlineKey(code: string): string | null {
  return HEADLINE_KEYS[code] ?? null;
}

/** Catalog key for one reason's corrective action. */
export function transitionReasonKey(rule: string): string {
  return REASON_KEYS[rule] ?? GENERIC_REASON_KEY;
}
