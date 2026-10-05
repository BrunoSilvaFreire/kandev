import type { TFunction } from "i18next";

/** Closed-set routing outcome to literal translation key (never parsed as copy). */
export function routingOutcomeLabel(t: TFunction, outcome: string): string {
  switch (outcome) {
    case "reused":
      return t("task:routingOutcomeReused");
    case "created":
      return t("task:routingOutcomeCreated");
    case "declined":
      return t("task:routingOutcomeDeclined");
    default:
      return outcome;
  }
}

/** Closed-set routing reason to literal translation key. */
export function routingReasonLabel(t: TFunction, reason: string): string {
  switch (reason) {
    case "reused_current_session":
      return t("task:routingReasonReusedCurrentSession");
    case "reused_existing":
      return t("task:routingReasonReusedExisting");
    case "step_primary":
      return t("task:routingReasonStepPrimary");
    case "unavailable_handoff":
      return t("task:routingReasonUnavailableHandoff");
    case "explicit_target":
      return t("task:routingReasonExplicitTarget");
    case "forced_new_policy":
      return t("task:routingReasonForcedNewPolicy");
    case "no_reusable_candidate":
      return t("task:routingReasonNoReusableCandidate");
    case "exact_model_incompatibility":
      return t("task:routingReasonExactModelIncompatibility");
    case "selected_candidate_terminal":
      return t("task:routingReasonSelectedCandidateTerminal");
    default:
      return reason;
  }
}
