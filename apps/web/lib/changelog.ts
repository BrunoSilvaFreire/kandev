export type ChangelogEntry = {
  version: string;
  date: string;
  notes: string;
};

let cached: ChangelogEntry[] | null = null;

/**
 * Loads the generated changelog on demand. The JSON is ~325 KB, so it is
 * imported dynamically to keep it out of the boot graph; every consumer that
 * needs it is a dialog or settings surface, never the initial task page.
 */
export async function loadChangelog(): Promise<ChangelogEntry[]> {
  if (cached) return cached;
  const module = await import("@/generated/changelog.json");
  cached = module.default as ChangelogEntry[];
  return cached;
}
