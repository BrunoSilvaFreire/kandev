import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  acknowledgeTriageItems,
  getAcknowledgedTriageIds,
  pruneAcknowledgedTriage,
  resetAcknowledgedTriageForTests,
  subscribeAcknowledgedTriage,
} from "./acknowledged-store";

function ids(): string[] {
  return [...getAcknowledgedTriageIds()].sort();
}

describe("acknowledged triage store", () => {
  beforeEach(() => {
    window.localStorage.clear();
    resetAcknowledgedTriageForTests();
  });

  afterEach(() => {
    resetAcknowledgedTriageForTests();
  });

  it("persists acknowledged identities and re-reads them after a reset", () => {
    acknowledgeTriageItems(["a", "b"]);
    expect(ids()).toEqual(["a", "b"]);

    resetAcknowledgedTriageForTests();
    expect(ids()).toEqual(["a", "b"]);
  });

  it("replaces the acknowledged set with the supplied identities", () => {
    acknowledgeTriageItems(["a", "b"]);
    acknowledgeTriageItems(["b", "c"]);
    expect(ids()).toEqual(["b", "c"]);
  });

  it("prunes acknowledgements whose identity is no longer surfaced", () => {
    acknowledgeTriageItems(["a", "b"]);
    pruneAcknowledgedTriage(new Set(["b"]));
    expect(ids()).toEqual(["b"]);
  });

  it("notifies subscribers on each change", () => {
    const listener = vi.fn();
    const unsubscribe = subscribeAcknowledgedTriage(listener);
    acknowledgeTriageItems(["a"]);
    expect(listener).toHaveBeenCalledTimes(1);
    pruneAcknowledgedTriage(new Set());
    expect(listener).toHaveBeenCalledTimes(2);
    unsubscribe();
    acknowledgeTriageItems(["b"]);
    expect(listener).toHaveBeenCalledTimes(2);
  });
});
