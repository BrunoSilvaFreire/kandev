import { cleanup, render, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  overview: {
    range: "7d",
    truncated: false,
    indexing: { state: "done", files_total: 0, files_done: 0 },
    sources: [{ source: "kandev", enabled: true, toggleable: false }],
    providers: [{ provider: "mock", status: "nearing", accounts: [] }],
  },
}));

vi.mock("@/hooks/domains/usage/use-provider-usage", () => ({
  useProviderUsage: () => ({
    overview: mocks.overview,
    loading: false,
    error: false,
    refresh: () => {},
  }),
}));

vi.mock("@/hooks/domains/usage/use-usage-index", () => ({
  useUsageIndex: () => ({ status: { state: "done" }, reindex: () => {}, refresh: () => {} }),
}));

vi.mock("@/lib/api/domains/provider-usage-api", () => ({
  updateUsageSources: vi.fn(),
}));

vi.mock("@/components/page-shell", () => ({
  PageShell: ({ title, subtitle, actions, children }: Record<string, React.ReactNode>) => (
    <section>
      <h1>{title}</h1>
      {subtitle && <p>{subtitle}</p>}
      {actions}
      {children}
    </section>
  ),
}));

import { UsagePageClient } from "./usage-page";
import {
  getProviderUsageAttention,
  setProviderUsageAttention,
} from "@/lib/state/provider-usage-attention";

beforeEach(() => setProviderUsageAttention(false));
afterEach(() => {
  cleanup();
  setProviderUsageAttention(false);
});

describe("UsagePageClient attention flag", () => {
  it("keeps the attention flag after the page unmounts", async () => {
    const { unmount } = render(<UsagePageClient />);
    await waitFor(() => expect(getProviderUsageAttention()).toBe(true));

    unmount();
    expect(getProviderUsageAttention()).toBe(true);
  });
});
