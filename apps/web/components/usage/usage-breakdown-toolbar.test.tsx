import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import type { UsageBreakdownFacets, UsageBreakdownQuery } from "@/lib/types/provider-usage";
import { UsageBreakdownToolbar } from "./usage-breakdown-toolbar";

afterEach(cleanup);

const noop = vi.fn();

const noFacets: UsageBreakdownFacets = { providers: [], models: [], agent_types: [] };

function renderToolbar(facets: UsageBreakdownFacets, query: UsageBreakdownQuery = {}) {
  return render(
    <UsageBreakdownToolbar query={query} setQuery={noop} facets={facets} isMobile={false} />,
  );
}

describe("UsageBreakdownToolbar", () => {
  it("renders a facet select when options are present", () => {
    renderToolbar({ ...noFacets, providers: ["openai"] });
    expect(screen.getByTestId("usage-breakdown-provider")).toBeTruthy();
  });

  it("drops blank facet options instead of mounting a Select.Item with an empty value", () => {
    renderToolbar({ ...noFacets, providers: [""] });
    expect(screen.queryByTestId("usage-breakdown-provider")).toBeNull();
  });

  it("keeps real options while dropping blanks", () => {
    renderToolbar({ ...noFacets, providers: ["", "anthropic"] });
    expect(screen.getByTestId("usage-breakdown-provider")).toBeTruthy();
  });
});
