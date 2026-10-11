import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import type { InboxTriageItem } from "@/lib/inbox-triage/possible-question-triage";

vi.mock("@/components/routing/app-link", () => ({
  default: ({ href, children, ...rest }: { href: string; children: ReactNode }) => (
    <a href={href} {...rest}>
      {children}
    </a>
  ),
}));

// The panel always evaluates its store-backed fallback; the tests drive it
// through the explicit controller instead.
vi.mock("@/hooks/domains/inbox-triage/use-inbox-triage", () => ({
  useInboxTriageNudge: () => ({
    items: [],
    newCount: 0,
    additionalCount: 0,
    acknowledgeAll: () => {},
  }),
}));

import { TriageTabPanel } from "./triage-tab-panel";

function item(overrides: Partial<InboxTriageItem>): InboxTriageItem {
  return {
    id: "interrupted:t1:e1",
    kind: "interrupted",
    reason: "backend_restart",
    taskId: "t1",
    sessionId: "s1",
    title: "Interrupted task",
    since: null,
    ...overrides,
  };
}

function renderPanel(items: InboxTriageItem[]) {
  const acknowledgeAll = vi.fn();
  render(
    <TriageTabPanel controller={{ items, newCount: 0, additionalCount: 0, acknowledgeAll }} />,
  );
  return acknowledgeAll;
}

describe("TriageTabPanel", () => {
  afterEach(cleanup);

  it("explains an interruption and offers Resume on the owning session", () => {
    const acknowledgeAll = renderPanel([item({})]);
    const row = screen.getByTestId("inbox-triage-row-interrupted");
    expect(row.getAttribute("href")).toContain("t1");
    expect(row.getAttribute("href")).toContain("s1");
    expect(screen.getByTestId("inbox-triage-reason").textContent).toContain("restarted");
    expect(screen.getByTestId("inbox-triage-action").textContent).toBe("Resume");
    expect(acknowledgeAll).toHaveBeenCalled();
  });

  it("gives a genuine review a Review action and its own reason", () => {
    renderPanel([item({ id: "review:t2:r1", kind: "review", reason: "review", taskId: "t2" })]);
    expect(screen.getByTestId("inbox-triage-row-review")).toBeTruthy();
    expect(screen.getByTestId("inbox-triage-reason").textContent).toContain(
      "waiting for your review",
    );
    expect(screen.getByTestId("inbox-triage-action").textContent).toBe("Review");
  });

  it("renders the empty state when nothing needs attention", () => {
    renderPanel([]);
    expect(screen.getByTestId("inbox-triage-empty")).toBeTruthy();
  });
});
