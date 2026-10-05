import { describe, expect, it } from "vitest";
import { surfaceCapabilities } from "./surface-capabilities";

describe("surfaceCapabilities", () => {
  it("enables every Task-only capability on a task surface", () => {
    expect(surfaceCapabilities("task")).toEqual({
      workflow: true,
      taskActions: true,
      pullRequests: true,
    });
  });

  it("disables every Task-only capability on a Quick Chat surface", () => {
    expect(surfaceCapabilities("quick-chat")).toEqual({
      workflow: false,
      taskActions: false,
      pullRequests: false,
    });
  });
});
