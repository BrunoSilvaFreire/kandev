import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { MessageSearchHit } from "@/lib/api/domains/session-api";
import { SessionSearchHits } from "./session-search-hits";

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      kanban: { steps: [{ id: "step-plan", title: "Plan" }] },
      kanbanMulti: { snapshots: {} },
    }),
}));

afterEach(() => cleanup());

function hit(overrides: Partial<MessageSearchHit>): MessageSearchHit {
  return {
    id: "m1",
    author_type: "agent",
    type: "text",
    snippet: "needle body",
    created_at: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

describe("SessionSearchHits task scope", () => {
  it("labels session and turn step provenance for cross-session hits", () => {
    render(
      <SessionSearchHits
        hits={[
          hit({
            id: "m1",
            session_id: "sess-2",
            session_name: "Session Beta",
            workflow_step_id: "step-plan",
          }),
        ]}
        query="needle"
        activeHitId={null}
        onSelect={vi.fn()}
        isSearching={false}
        taskScoped
        currentSessionId="sess-1"
      />,
    );
    expect(screen.getByText("Session: Session Beta")).toBeTruthy();
    expect(screen.getByText("Step: Plan")).toBeTruthy();
    expect(screen.getByTestId("search-hit-cross-session-m1")).toBeTruthy();
  });

  it("omits the cross-session marker for the current session and unknown steps", () => {
    render(
      <SessionSearchHits
        hits={[hit({ id: "m1", session_id: "sess-1", workflow_step_id: null })]}
        query="needle"
        activeHitId={null}
        onSelect={vi.fn()}
        isSearching={false}
        taskScoped
        currentSessionId="sess-1"
      />,
    );
    expect(screen.queryByTestId("search-hit-cross-session-m1")).toBeNull();
    expect(screen.queryByText(/^Step:/)).toBeNull();
  });

  it("requests the next page when more results remain", () => {
    const onLoadMore = vi.fn();
    render(
      <SessionSearchHits
        hits={[hit({ id: "m1" })]}
        query="needle"
        activeHitId={null}
        onSelect={vi.fn()}
        isSearching={false}
        taskScoped
        hasMore
        onLoadMore={onLoadMore}
      />,
    );
    fireEvent.click(screen.getByTestId("search-load-more"));
    expect(onLoadMore).toHaveBeenCalled();
  });

  it("selects the hit by id", () => {
    const onSelect = vi.fn();
    render(
      <SessionSearchHits
        hits={[hit({ id: "m1" })]}
        query="needle"
        activeHitId={null}
        onSelect={onSelect}
        isSearching={false}
      />,
    );
    fireEvent.click(screen.getAllByRole("button")[0]);
    expect(onSelect).toHaveBeenCalledWith("m1");
  });
});
