import { getWebSocketClient } from "@/lib/ws/connection";
import type {
  TaskDocumentCatalogGroup,
  TaskDocumentDetail,
  TaskDocumentRevision,
} from "@/lib/types/task-document";

// i18n-exempt: diagnostic thrown to the caller, never rendered.
const WS_CLIENT_UNAVAILABLE = "WebSocket client not available";

/** Server-side page size used when a caller does not request one. */
export const DEFAULT_DOCUMENT_REVISION_PAGE_SIZE = 20;

/**
 * List the merged, metadata-only task document catalog, grouped by the
 * producing session of each entry's latest revision. The authoritative Plan
 * is included by the server with `is_plan: true`.
 */
export async function listTaskDocumentCatalog(taskId: string): Promise<TaskDocumentCatalogGroup[]> {
  const client = getWebSocketClient();
  if (!client) {
    throw new Error(WS_CLIENT_UNAVAILABLE);
  }
  const response = await client.request("task.documents.catalog", { task_id: taskId });
  const groups = (response as { groups?: TaskDocumentCatalogGroup[] })?.groups;
  return groups ?? [];
}

/** Fetch a selected document's content and latest-revision provenance. */
export async function getTaskDocument(taskId: string, key: string): Promise<TaskDocumentDetail> {
  const client = getWebSocketClient();
  if (!client) {
    throw new Error(WS_CLIENT_UNAVAILABLE);
  }
  const response = await client.request("task.document.get", { task_id: taskId, key });
  return response as TaskDocumentDetail;
}

export type DocumentRevisionPage = {
  revisions: TaskDocumentRevision[];
  limit: number;
};

/**
 * Fetch one bounded page of revision metadata (newest-first, no bodies).
 * `beforeRevision` is exclusive and pages backward; the server clamps `limit`.
 */
export async function listTaskDocumentRevisions(
  taskId: string,
  key: string,
  options: { limit?: number; beforeRevision?: number } = {},
): Promise<DocumentRevisionPage> {
  const client = getWebSocketClient();
  if (!client) {
    throw new Error(WS_CLIENT_UNAVAILABLE);
  }
  const limit = options.limit ?? DEFAULT_DOCUMENT_REVISION_PAGE_SIZE;
  const payload: Record<string, string | number> = { task_id: taskId, key, limit };
  if (options.beforeRevision) payload.before_revision = options.beforeRevision;
  const response = await client.request("task.document.revisions.list", payload);
  const revisions = (response as { revisions?: TaskDocumentRevision[] })?.revisions;
  return { revisions: revisions ?? [], limit };
}
