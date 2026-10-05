import { describe, expect, it } from "vitest";
import { WELL_KNOWN_DOCUMENT_KEYS } from "./task-document";

describe("WELL_KNOWN_DOCUMENT_KEYS", () => {
  it("keeps the shared document-key conventions", () => {
    expect(WELL_KNOWN_DOCUMENT_KEYS).toEqual([
      "plan",
      "spec",
      "spike",
      "notes",
      "review",
      "handoff",
    ]);
  });
});
