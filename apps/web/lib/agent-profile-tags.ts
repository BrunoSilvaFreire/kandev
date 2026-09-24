// Canonicalization for free-form agent-profile tags and workflow-step allowed
// tags. Mirrors the backend shared canonicalizer: trim, lowercase, reject
// empty, deduplicate, sort, and bound count and per-tag size.

export const MAX_PROFILE_TAGS = 32;
export const MAX_PROFILE_TAG_SIZE = 64;

const textEncoder = new TextEncoder();

/** UTF-8 byte length, matching the backend's per-tag cap. */
export function tagByteLength(tag: string): number {
  return textEncoder.encode(tag).length;
}

export function canonicalizeTags(input: readonly string[]): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const raw of input) {
    const tag = raw.trim().toLowerCase();
    if (tag.length === 0 || tagByteLength(tag) > MAX_PROFILE_TAG_SIZE) continue;
    if (seen.has(tag)) continue;
    seen.add(tag);
    out.push(tag);
  }
  return out.sort().slice(0, MAX_PROFILE_TAGS);
}
