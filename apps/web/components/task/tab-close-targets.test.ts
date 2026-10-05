import { describe, expect, it } from "vitest";
import { closeTargets } from "./tab-close-targets";

const ORDER = ["chat", "session:a", "file:1", "session:b", "pr-detail"];

describe("closeTargets", () => {
  it("closes every sibling except the target and the structural chat panel", () => {
    expect(closeTargets(ORDER, "session:a", "others")).toEqual([
      "file:1",
      "session:b",
      "pr-detail",
    ]);
  });

  it("closes only the tabs after the target", () => {
    expect(closeTargets(ORDER, "session:a", "right")).toEqual(["file:1", "session:b", "pr-detail"]);
    expect(closeTargets(ORDER, "file:1", "right")).toEqual(["session:b", "pr-detail"]);
  });

  it("closes nothing when the target is rightmost", () => {
    expect(closeTargets(ORDER, "pr-detail", "right")).toEqual([]);
  });

  it("closes nothing for an unknown target", () => {
    expect(closeTargets(ORDER, "missing", "others")).toEqual([]);
    expect(closeTargets(ORDER, "missing", "right")).toEqual([]);
  });

  it("never returns the chat panel", () => {
    expect(closeTargets(ORDER, "session:a", "others")).not.toContain("chat");
    expect(closeTargets(ORDER, "chat", "right")).not.toContain("chat");
  });
});
