/**
 * Task document catalog types shared by the Documents panel and the Review
 * surface. These mirror the backend `document_catalog.go` DTOs; list responses
 * never carry a document body.
 */

export const PLAN_DOCUMENT_KEY = "plan";

/** Conventional task-document names. Custom keys are first-class too. */
export const WELL_KNOWN_DOCUMENT_KEYS = ["plan", "spec", "spike", "notes", "review", "handoff"];

/** One metadata-only catalog row (never has a body). */
export type TaskDocumentSummary = {
  key: string;
  title: string;
  type: string;
  is_plan: boolean;
  latest_revision_number: number;
  updated_at: string;
  source_session_id?: string;
  source_workflow_step_id?: string;
};

/** Catalog entries grouped by the session that produced their latest revision.
 *  A missing `session_id` is the Unknown/legacy group. */
export type TaskDocumentCatalogGroup = {
  session_id?: string;
  entries: TaskDocumentSummary[];
};

/** A selected document's content plus the latest revision's provenance. */
export type TaskDocumentDetail = {
  key: string;
  title: string;
  type: string;
  is_plan: boolean;
  content: string;
  latest_revision_number: number;
  updated_at: string;
  source_session_id?: string;
  source_workflow_step_id?: string;
};

/** One revision in a document's history. `content` is only present for a
 *  single-revision fetch, never in a list response. */
export type TaskDocumentRevision = {
  id: string;
  revision_number: number;
  title: string;
  content?: string;
  author_kind: string;
  author_name: string;
  source_session_id?: string;
  source_workflow_step_id?: string;
  created_at: string;
  updated_at: string;
};
