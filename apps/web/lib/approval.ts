import type { PlanComment } from "@/lib/state/slices/comments";
import type {
  ClarificationAnswer,
  ClarificationApprovalMeta,
  ClarificationApprovalOutcome,
  ClarificationRequestMetadata,
} from "@/lib/types/http";
import { toTaskPlanCommentRefs } from "@/lib/plan-comment-refs";

/** The fixed question id every approval bundle uses. */
export const APPROVAL_QUESTION_ID = "approval";

export type ApprovalDecision = ClarificationApprovalOutcome["decision"];

/** Reads the approval metadata off a clarification request, if present. */
export function approvalMeta(metadata: unknown): ClarificationApprovalMeta | undefined {
  const request = metadata as ClarificationRequestMetadata | undefined;
  return request?.approval;
}

/** True when a clarification bundle is a request_approval_kandev approval. */
export function isApprovalRequest(metadata: unknown): boolean {
  return approvalMeta(metadata) !== undefined;
}

/**
 * While an approval bundle is pending, its Revise answer owns the task's
 * pending plan comments. The composer must not auto-attach them to an
 * unrelated chat message in that window.
 */
export function shouldSuppressPlanCommentAutoAttach(pendingClarificationMetadata: unknown): boolean {
  return isApprovalRequest(pendingClarificationMetadata);
}

/**
 * Revise is enabled when there is feedback text, at least one pending plan
 * comment, or the subject was edited since the request.
 */
export function canSubmitRevise(args: {
  feedback: string;
  commentCount: number;
  subjectEdited: boolean;
}): boolean {
  return args.feedback.trim().length > 0 || args.commentCount > 0 || args.subjectEdited;
}

/**
 * The browser-visible "edited since request" signal compares the plan's current
 * `updated_at` with the approval message's `created_at`. The backend keeps its
 * own authoritative check (plan write version), so the two may differ only in
 * the tiny window between a write and the message timestamp.
 */
export function isSubjectEdited(
  planUpdatedAt: string | null | undefined,
  requestCreatedAt: string | null | undefined,
): boolean {
  if (!planUpdatedAt || !requestCreatedAt) return false;
  const plan = Date.parse(planUpdatedAt);
  const request = Date.parse(requestCreatedAt);
  if (Number.isNaN(plan) || Number.isNaN(request)) return false;
  return plan > request;
}

/**
 * Builds the clarification answer for a decision. Only a revise attaches plan
 * comment references, and only the pending comments it was given.
 */
export function buildApprovalAnswer(args: {
  decision: ApprovalDecision;
  feedback?: string;
  comments?: PlanComment[];
}): ClarificationAnswer {
  const answer: ClarificationAnswer = {
    question_id: APPROVAL_QUESTION_ID,
    selected_options: [args.decision],
  };
  const feedback = args.feedback?.trim();
  if (feedback) answer.custom_text = feedback;
  if (args.decision === "revise" && args.comments && args.comments.length > 0) {
    answer.plan_comment_refs = toTaskPlanCommentRefs(args.comments);
  }
  return answer;
}
