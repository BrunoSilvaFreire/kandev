import { describe, expect, it } from "vitest";
import type { PlanComment } from "@/lib/state/slices/comments";
import {
  APPROVAL_QUESTION_ID,
  approvalMeta,
  buildApprovalAnswer,
  canSubmitRevise,
  isApprovalRequest,
  isSubjectEdited,
  shouldSuppressPlanCommentAutoAttach,
} from "@/lib/approval";

function planComment(id: string, version: number): PlanComment {
  return {
    id,
    sessionId: "s1",
    taskId: "t1",
    planId: "p1",
    type: "plan",
    body: "fix this",
    selectedText: "x",
    anchorFrom: 0,
    anchorTo: 1,
    version,
    createdAt: "2026-09-24T00:00:00Z",
    updatedAt: "2026-09-24T00:00:00Z",
  } as unknown as PlanComment;
}

describe("approval metadata helpers", () => {
  it("detects an approval bundle", () => {
    const metadata = {
      pending_id: "p",
      question: { id: "approval", title: "Approve", prompt: "?", options: [] },
      approval: { subject: "task_plan", title: "Plan revision 2" },
    };
    expect(isApprovalRequest(metadata)).toBe(true);
    expect(approvalMeta(metadata)?.subject).toBe("task_plan");
  });

  it("rejects a normal question bundle", () => {
    expect(isApprovalRequest({ pending_id: "p", question: {} })).toBe(false);
    expect(isApprovalRequest(undefined)).toBe(false);
  });
});

describe("composer plan-comment auto-attach suppression", () => {
  const approvalMetadata = {
    pending_id: "p",
    question: { id: "approval", title: "Approve", prompt: "?", options: [] },
    approval: { subject: "task_plan", title: "Plan revision 2" },
  };

  it("suppresses auto-attach while an approval is pending", () => {
    expect(shouldSuppressPlanCommentAutoAttach(approvalMetadata)).toBe(true);
  });

  it("does not suppress for a normal clarification", () => {
    expect(shouldSuppressPlanCommentAutoAttach({ pending_id: "p", question: {} })).toBe(false);
    expect(shouldSuppressPlanCommentAutoAttach(undefined)).toBe(false);
  });
});

describe("canSubmitRevise", () => {
  it("is enabled by any of feedback, comments, or an edit", () => {
    expect(canSubmitRevise({ feedback: "fix it", commentCount: 0, subjectEdited: false })).toBe(true);
    expect(canSubmitRevise({ feedback: "  ", commentCount: 2, subjectEdited: false })).toBe(true);
    expect(canSubmitRevise({ feedback: "", commentCount: 0, subjectEdited: true })).toBe(true);
  });

  it("is disabled when nothing changed", () => {
    expect(canSubmitRevise({ feedback: "  ", commentCount: 0, subjectEdited: false })).toBe(false);
  });
});

const REQUEST_AT = "2026-09-24T01:00:00Z";

describe("isSubjectEdited", () => {
  it("compares the plan timestamp against the request timestamp", () => {
    expect(isSubjectEdited("2026-09-24T02:00:00Z", REQUEST_AT)).toBe(true);
    expect(isSubjectEdited(REQUEST_AT, REQUEST_AT)).toBe(false);
    expect(isSubjectEdited("2026-09-24T00:00:00Z", REQUEST_AT)).toBe(false);
  });

  it("is false for missing or malformed timestamps", () => {
    expect(isSubjectEdited(null, REQUEST_AT)).toBe(false);
    expect(isSubjectEdited(REQUEST_AT, undefined)).toBe(false);
    expect(isSubjectEdited("not-a-date", REQUEST_AT)).toBe(false);
  });
});

describe("buildApprovalAnswer", () => {
  it("builds approve without refs even when comments are present", () => {
    const answer = buildApprovalAnswer({
      decision: "approve",
      comments: [planComment("c1", 3)],
    });
    expect(answer).toEqual({ question_id: APPROVAL_QUESTION_ID, selected_options: ["approve"] });
  });

  it("attaches only pending refs on revise", () => {
    const answer = buildApprovalAnswer({
      decision: "revise",
      feedback: "please fix",
      comments: [planComment("c1", 3), planComment("c2", 4)],
    });
    expect(answer.question_id).toBe(APPROVAL_QUESTION_ID);
    expect(answer.selected_options).toEqual(["revise"]);
    expect(answer.custom_text).toBe("please fix");
    expect(answer.plan_comment_refs).toEqual([
      { id: "c1", version: 3 },
      { id: "c2", version: 4 },
    ]);
  });

  it("omits empty feedback and refs on revise", () => {
    const answer = buildApprovalAnswer({ decision: "revise", feedback: "   ", comments: [] });
    expect(answer.custom_text).toBeUndefined();
    expect(answer.plan_comment_refs).toBeUndefined();
  });
});
