import { describe, expect, it } from "vitest";
import { MAX_PROFILE_TAGS, MAX_PROFILE_TAG_SIZE, canonicalizeTags } from "./agent-profile-tags";

describe("canonicalizeTags", () => {
  it("trims, lowercases, dedupes, and sorts", () => {
    expect(canonicalizeTags([" Review ", "SECURITY", "review"])).toEqual(["review", "security"]);
  });

  it("drops empty and oversized tokens", () => {
    expect(canonicalizeTags(["", "  ", "a".repeat(65), "ok"])).toEqual(["ok"]);
  });

  it("caps the list", () => {
    const many = Array.from({ length: MAX_PROFILE_TAGS + 5 }, (_, i) => `tag-${i}`);
    expect(canonicalizeTags(many)).toHaveLength(MAX_PROFILE_TAGS);
  });

  it("measures the per-tag cap in UTF-8 bytes, not UTF-16 units", () => {
    // "é" is 2 UTF-8 bytes; 40 of them are 80 bytes but only 40 code units.
    const overBytes = "é".repeat(40);
    expect(overBytes.length).toBeLessThanOrEqual(MAX_PROFILE_TAG_SIZE);
    expect(canonicalizeTags([overBytes])).toEqual([]);

    const exactly64Bytes = "é".repeat(32);
    expect(canonicalizeTags([exactly64Bytes])).toEqual([exactly64Bytes]);
  });
});
