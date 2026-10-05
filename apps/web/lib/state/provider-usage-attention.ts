// Process-local attention flag for the sidebar Usage item. The page writes the
// last fetched overview's worst status here; the sidebar only reads it, so the
// sidebar never polls. Nothing is persisted.

let attention = false;
const listeners = new Set<() => void>();

export function setProviderUsageAttention(next: boolean): void {
  if (next === attention) return;
  attention = next;
  for (const listener of listeners) listener();
}

export function getProviderUsageAttention(): boolean {
  return attention;
}

export function subscribeProviderUsageAttention(listener: () => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}
