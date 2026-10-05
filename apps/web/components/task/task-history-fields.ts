import type { TFunction } from "i18next";
import type { TaskActivityEvent } from "@/lib/api/domains/task-activity-api";
import { routingOutcomeLabel, routingReasonLabel } from "./task-history-labels";

export type HistoryNames = {
  stepName: string | null;
  profileName: string | null;
  sourceSessionName: string | null;
  destinationSessionName: string | null;
};

export type HistoryField = { label: string; value: string };

// Only closed keys are rendered; an unrecognised key is ignored rather than
// shown as raw JSON.
const DECISION_DETAIL_LABELS: Record<string, string> = {
  candidate_session_id: "task:taskHistoryFieldCandidateSession",
  candidate_state: "task:taskHistoryFieldCandidateState",
  required_model: "task:taskHistoryFieldRequiredModel",
  candidate_model: "task:taskHistoryFieldCandidateModel",
  start_policy: "task:taskHistoryFieldStartPolicy",
};

function decisionDetailFields(t: TFunction, raw: string | undefined): HistoryField[] {
  if (!raw) return [];
  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return [];
  }
  if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) return [];
  const record = parsed as Record<string, unknown>;
  const fields: HistoryField[] = [];
  for (const [key, labelKey] of Object.entries(DECISION_DETAIL_LABELS)) {
    const value = record[key];
    if (typeof value === "string" && value !== "") fields.push({ label: t(labelKey), value });
  }
  return fields;
}

function policyPair(t: TFunction, startPolicy?: string, endPolicy?: string): string | null {
  const parts: string[] = [];
  if (startPolicy) parts.push(t("task:taskHistoryPolicyStart", { policy: startPolicy }));
  if (endPolicy) parts.push(t("task:taskHistoryPolicyEnd", { policy: endPolicy }));
  return parts.length > 0 ? parts.join(", ") : null;
}

function sessionPair(t: TFunction, names: HistoryNames): string | null {
  if (!names.sourceSessionName && !names.destinationSessionName) return null;
  const unknown = t("task:taskHistoryUnknownSession");
  return `${names.sourceSessionName ?? unknown} -> ${names.destinationSessionName ?? unknown}`;
}

/** Route explanation line: outcome, reason, destination step, policies, profile, sessions. */
export function routeExplanation(
  t: TFunction,
  event: TaskActivityEvent,
  names: HistoryNames,
): string | null {
  const route = event.route;
  if (!route) return null;
  const parts = [
    routingOutcomeLabel(t, route.outcome),
    routingReasonLabel(t, route.reason),
    names.stepName,
    policyPair(t, route.start_policy, route.end_policy),
    names.profileName,
    sessionPair(t, names),
  ].filter((part): part is string => Boolean(part));
  return parts.length > 0 ? parts.join(" \u00b7 ") : null;
}

/** Transition explanation line: trigger, trigger detail, actor. */
export function transitionExplanation(t: TFunction, event: TaskActivityEvent): string | null {
  const transition = event.transition;
  if (!transition) return null;
  const parts = [transition.trigger, transition.trigger_detail, transition.actor_kind].filter(
    Boolean,
  );
  return parts.length > 0 ? parts.join(" \u00b7 ") : null;
}

/** The raw closed-set fields for the expandable Details block. */
export function rawHistoryFields(
  t: TFunction,
  event: TaskActivityEvent,
  names: HistoryNames,
): HistoryField[] {
  const fields: HistoryField[] = [];
  const route = event.route;
  if (route) {
    fields.push({ label: t("task:taskHistoryFieldOutcome"), value: route.outcome });
    fields.push({ label: t("task:taskHistoryFieldReason"), value: route.reason });
    if (route.destination_step_id) {
      fields.push({
        label: t("task:taskHistoryFieldDestinationStep"),
        value: route.destination_step_id,
      });
    }
    if (route.start_policy) {
      fields.push({ label: t("task:taskHistoryFieldStartPolicy"), value: route.start_policy });
    }
    if (route.end_policy) {
      fields.push({ label: t("task:taskHistoryFieldEndPolicy"), value: route.end_policy });
    }
    if (names.profileName) {
      fields.push({ label: t("task:taskHistoryFieldProfile"), value: names.profileName });
    }
    if (route.source_session_id) {
      fields.push({
        label: t("task:taskHistoryFieldSourceSession"),
        value: names.sourceSessionName ?? route.source_session_id,
      });
    }
    if (route.destination_session_id) {
      fields.push({
        label: t("task:taskHistoryFieldDestinationSession"),
        value: names.destinationSessionName ?? route.destination_session_id,
      });
    }
    fields.push(...decisionDetailFields(t, route.decision_detail));
  }
  const transition = event.transition;
  if (transition) {
    fields.push({ label: t("task:taskHistoryFieldTrigger"), value: transition.trigger });
    if (transition.trigger_detail) {
      fields.push({
        label: t("task:taskHistoryFieldTriggerDetail"),
        value: transition.trigger_detail,
      });
    }
    fields.push({ label: t("task:taskHistoryFieldActor"), value: transition.actor_kind });
  }
  return fields.filter((field) => field.value !== "");
}
