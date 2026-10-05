import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { TaskActivityEvent } from "@/lib/api/domains/task-activity-api";
import { TaskHistoryPanel } from "./task-history-panel";

const activity = vi.hoisted(() => ({
  value: {
    events: [] as TaskActivityEvent[],
    status: "success",
    hasMore: false,
    loadMore: vi.fn(),
    reload: vi.fn(),
  },
}));

vi.mock("@/hooks/domains/task/use-task-activity", () => ({
  useTaskActivity: () => activity.value,
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      kanban: { steps: [{ id: "step-plan", title: "Plan" }] },
      kanbanMulti: { snapshots: {} },
      agentProfiles: { items: [{ id: "profile-1", label: "Claude Opus" }] },
      taskSessions: { items: { "sess-dest": { id: "sess-dest", name: "Destination" } } },
    }),
}));

afterEach(() => cleanup());

const AT = "2026-07-01T00:00:00Z";

function base(overrides: Partial<TaskActivityEvent>): TaskActivityEvent {
  return {
    kind: "transition",
    id: "e1",
    occurred_at: AT,
    ...overrides,
  };
}

describe("TaskHistoryPanel", () => {
  it("renders a route row with localized reason and Open for the destination", () => {
    const onOpenSession = vi.fn();
    activity.value = {
      events: [
        base({
          kind: "route",
          id: "r1",
          route: {
            id: "r1",
            destination_step_id: "step-impl",
            destination_session_id: "sess-dest",
            agent_profile_id: "profile-1",
            outcome: "reused",
            reason: "reused_existing",
            created_at: AT,
          },
        }),
      ],
      status: "success",
      hasMore: false,
      loadMore: vi.fn(),
      reload: vi.fn(),
    };
    render(<TaskHistoryPanel taskId="task-1" onOpenSession={onOpenSession} />);
    expect(screen.getByTestId("task-history-row-route").textContent).toContain("Reused session");
    expect(screen.getByTestId("task-history-row-route").textContent).toContain(
      "Reused an existing session",
    );
    fireEvent.click(screen.getByTestId("task-history-open-r1"));
    expect(onOpenSession).toHaveBeenCalledWith("sess-dest");
  });

  it("names the profile and expands raw route details", () => {
    activity.value = {
      events: [
        base({
          kind: "route",
          id: "r3",
          route: {
            id: "r3",
            destination_step_id: "step-impl",
            source_session_id: "sess-src",
            destination_session_id: "sess-dest",
            agent_profile_id: "profile-1",
            start_policy: "reuse",
            end_policy: "stop",
            outcome: "reused",
            reason: "reused_existing",
            decision_detail: JSON.stringify({
              candidate_session_id: "candidate-7",
              candidate_state: "WAITING_FOR_INPUT",
            }),
            created_at: AT,
          },
        }),
      ],
      status: "success",
      hasMore: false,
      loadMore: vi.fn(),
      reload: vi.fn(),
    };
    render(<TaskHistoryPanel taskId="task-1" />);
    const row = screen.getByTestId("task-history-row-route");
    // The explanation uses the profile label, never the id.
    expect(row.textContent).toContain("Claude Opus");
    expect(row.textContent).not.toContain("profile-1");
    expect(row.textContent).toContain("reuse");

    fireEvent.click(screen.getByTestId("task-history-details").querySelector("summary")!);
    const details = screen.getByTestId("task-history-details");
    expect(details.textContent).toContain("reused_existing");
    expect(details.textContent).toContain("reuse");
    expect(details.textContent).toContain("Claude Opus");
    expect(details.textContent).toContain("candidate-7");
    expect(details.textContent).toContain("WAITING_FOR_INPUT");
  });
});

describe("TaskHistoryPanel visits and reviews", () => {
  it("renders a step visit with its routed session and Open for the destination", () => {
    const onOpenSession = vi.fn();
    activity.value = {
      events: [
        base({
          kind: "transition",
          id: "transition:7",
          transition: {
            id: 7,
            to_step_id: "step-plan",
            trigger: "turn_complete",
            actor_kind: "system",
            occurred_at: AT,
          },
          route: {
            id: "r7",
            destination_step_id: "step-plan",
            destination_session_id: "sess-visit",
            agent_profile_id: "profile-1",
            outcome: "created",
            reason: "no_reusable_candidate",
            created_at: AT,
          },
        }),
      ],
      status: "success",
      hasMore: false,
      loadMore: vi.fn(),
      reload: vi.fn(),
    };
    render(<TaskHistoryPanel taskId="task-1" onOpenSession={onOpenSession} />);
    const row = screen.getByTestId("task-history-row-transition");
    expect(row.textContent).toContain("Created session");
    expect(row.textContent).toContain("No reusable session candidate");
    fireEvent.click(screen.getByTestId("task-history-open-transition:7"));
    expect(onOpenSession).toHaveBeenCalledWith("sess-visit");
  });

  it("renders a declined route without Open and a sessionless transition without Open", () => {
    activity.value = {
      events: [
        base({
          kind: "route",
          id: "r2",
          route: {
            id: "r2",
            destination_step_id: "step-impl",
            outcome: "declined",
            reason: "exact_model_incompatibility",
            created_at: AT,
          },
        }),
        base({
          kind: "transition",
          id: "t1",
          transition: {
            id: 1,
            trigger: "manual_move",
            actor_kind: "user",
            occurred_at: "2026-07-01T00:00:00Z",
          },
        }),
      ],
      status: "success",
      hasMore: false,
      loadMore: vi.fn(),
      reload: vi.fn(),
    };
    render(<TaskHistoryPanel taskId="task-1" onOpenSession={vi.fn()} />);
    expect(screen.queryByTestId("task-history-open-r2")).toBeNull();
    expect(screen.getByTestId("task-history-row-transition").textContent).not.toContain("Open");
  });
});

describe("TaskHistoryPanel states", () => {
  it("reviews a document revision row and pages with the cursor", () => {
    const onReview = vi.fn();
    const loadMore = vi.fn();
    activity.value = {
      events: [
        base({
          kind: "document_revision",
          id: "d1",
          document_revision: {
            id: "d1",
            document_key: "architecture",
            revision_number: 4,
            title: "Architecture",
            author_kind: "agent",
            author_name: "Agent",
            created_at: AT,
          },
        }),
      ],
      status: "success",
      hasMore: true,
      loadMore,
      reload: vi.fn(),
    };
    render(<TaskHistoryPanel taskId="task-1" onReview={onReview} />);
    fireEvent.click(screen.getByTestId("task-history-review-architecture"));
    expect(onReview).toHaveBeenCalledWith("architecture");
    fireEvent.click(screen.getByTestId("task-history-load-more"));
    expect(loadMore).toHaveBeenCalled();
  });

  it("shows retry on failure and empty state", () => {
    activity.value = {
      events: [],
      status: "error",
      hasMore: false,
      loadMore: vi.fn(),
      reload: vi.fn(),
    };
    const { unmount } = render(<TaskHistoryPanel taskId="task-1" />);
    expect(screen.getByText("Failed to load task history.")).toBeTruthy();
    unmount();
    activity.value = {
      events: [],
      status: "success",
      hasMore: false,
      loadMore: vi.fn(),
      reload: vi.fn(),
    };
    render(<TaskHistoryPanel taskId="task-1" />);
    expect(screen.getByText("No task history yet")).toBeTruthy();
  });
});
