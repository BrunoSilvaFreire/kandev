import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { WorkflowStep } from "@/lib/types/http";
import { WorkflowStepAllowedTags } from "./workflow-step-allowed-tags";

const breakpoint = { isMobile: false };

type UtilItem = { profile_id: string; state: string; remaining_pct?: number };

const utilization = vi.fn();
const knownItems: Record<string, UtilItem> = {
  "profile-a": { profile_id: "profile-a", state: "known", remaining_pct: 72 },
};

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => breakpoint,
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      settingsAgents: {
        items: [
          {
            id: "agent-a",
            profiles: [
              { id: "profile-a", name: "Claude • Reviewer", enabled: true, tags: ["review"] },
            ],
          },
        ],
      },
    }),
}));

vi.mock("@/hooks/domains/settings/use-agent-profile-utilization", () => ({
  useAgentProfileUtilization: (ids: string[], enabled: boolean) => {
    utilization(ids, enabled);
    return { items: knownItems, loading: false, error: null };
  },
}));

const step = {
  id: "step-1",
  workflow_id: "workflow-1",
  name: "Review",
  position: 0,
  color: "bg-blue-500",
  created_at: "",
  updated_at: "",
  allowed_tags: ["review"],
} as WorkflowStep;

function renderAllowedTags(overrides: Partial<WorkflowStep> = {}, readOnly = false) {
  const onUpdate = vi.fn();
  render(
    <WorkflowStepAllowedTags
      step={{ ...step, ...overrides }}
      onUpdate={onUpdate}
      readOnly={readOnly}
    />,
  );
  return { onUpdate };
}

beforeEach(() => {
  breakpoint.isMobile = false;
  utilization.mockClear();
});

afterEach(cleanup);

describe("WorkflowStepAllowedTags", () => {
  it("shows the candidate preview and loads quota even when read-only", () => {
    renderAllowedTags(undefined, true);

    expect(screen.getByTestId("step-1-candidate-preview")).toBeTruthy();
    expect(screen.getByText("72% remaining")).toBeTruthy();
    expect(utilization).toHaveBeenCalledWith(["profile-a"], true);
  });

  it("keeps the tag input disabled while read-only", () => {
    renderAllowedTags(undefined, true);

    expect((screen.getByTestId("step-1-allowed-tags-input") as HTMLInputElement).disabled).toBe(
      true,
    );
  });

  it("hides the preview and skips quota when a session target conflicts", () => {
    renderAllowedTags({ session_target: { kind: "initial" } });

    expect(screen.queryByTestId("step-1-candidate-preview")).toBeNull();
    expect(utilization).toHaveBeenCalledWith(["profile-a"], false);
  });

  it("opens the candidate picker sheet on phones", () => {
    breakpoint.isMobile = true;
    renderAllowedTags();

    fireEvent.click(screen.getByTestId("step-1-candidate-preview-trigger"));
    expect(screen.getByTestId("step-1-candidate-preview-sheet-content")).toBeTruthy();
    expect(screen.getByText("72% remaining")).toBeTruthy();
  });
});
