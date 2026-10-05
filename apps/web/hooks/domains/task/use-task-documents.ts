"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import {
  DEFAULT_DOCUMENT_REVISION_PAGE_SIZE,
  getTaskDocument,
  listTaskDocumentCatalog,
  listTaskDocumentRevisions,
} from "@/lib/api/domains/task-document-api";
import type {
  TaskDocumentCatalogGroup,
  TaskDocumentDetail,
  TaskDocumentRevision,
} from "@/lib/types/task-document";

export type DocumentLoadStatus = "idle" | "loading" | "success" | "error";

const EMPTY_GROUPS: TaskDocumentCatalogGroup[] = [];
const EMPTY_REVISIONS: TaskDocumentRevision[] = [];

/** Load the grouped, metadata-only document catalog for a task. */
export function useTaskDocumentsCatalog(taskId: string | null | undefined, enabled = true) {
  const connectionStatus = useAppStore((state) => state.connection.status);
  const [groups, setGroups] = useState<TaskDocumentCatalogGroup[]>(EMPTY_GROUPS);
  const [status, setStatus] = useState<DocumentLoadStatus>("idle");
  const [reloadToken, setReloadToken] = useState(0);
  const requestRef = useRef(0);
  const key = taskId ?? null;

  useEffect(() => {
    const requestId = ++requestRef.current;
    if (!enabled || !key) {
      setGroups(EMPTY_GROUPS);
      setStatus("idle");
      return;
    }
    setStatus("loading");
    listTaskDocumentCatalog(key)
      .then((next) => {
        if (requestRef.current !== requestId) return;
        setGroups(next);
        setStatus("success");
      })
      .catch(() => {
        if (requestRef.current !== requestId) return;
        setGroups(EMPTY_GROUPS);
        setStatus("error");
      });
    return () => {
      if (requestRef.current === requestId) requestRef.current += 1;
    };
  }, [enabled, key, connectionStatus, reloadToken]);

  const reload = useCallback(() => setReloadToken((token) => token + 1), []);

  return { groups, status, reload };
}

export type TaskDocumentReviewState = {
  detail: TaskDocumentDetail | null;
  status: DocumentLoadStatus;
  revisions: TaskDocumentRevision[];
  revisionsStatus: DocumentLoadStatus;
  hasMoreRevisions: boolean;
  loadMoreRevisions: () => void;
  reload: () => void;
};

/**
 * Load a single document's content and its revision history. Catalog loading
 * is deliberately not part of this hook; the Review surface never needs the
 * catalog. Revision pagination pages backward using the oldest loaded
 * revision number as the exclusive cursor.
 */
export function useTaskDocumentReview(
  taskId: string | null | undefined,
  documentKey: string | null | undefined,
  enabled = true,
): TaskDocumentReviewState {
  const connectionStatus = useAppStore((state) => state.connection.status);
  const [detail, setDetail] = useState<TaskDocumentDetail | null>(null);
  const [status, setStatus] = useState<DocumentLoadStatus>("idle");
  const [revisions, setRevisions] = useState<TaskDocumentRevision[]>(EMPTY_REVISIONS);
  const [revisionsStatus, setRevisionsStatus] = useState<DocumentLoadStatus>("idle");
  const [hasMoreRevisions, setHasMoreRevisions] = useState(false);
  const [reloadToken, setReloadToken] = useState(0);
  const detailRequestRef = useRef(0);
  const revisionsRequestRef = useRef(0);

  useEffect(() => {
    const requestId = ++detailRequestRef.current;
    if (!enabled || !taskId || !documentKey) {
      setDetail(null);
      setStatus("idle");
      return;
    }
    setStatus("loading");
    getTaskDocument(taskId, documentKey)
      .then((next) => {
        if (detailRequestRef.current !== requestId) return;
        setDetail(next);
        setStatus("success");
      })
      .catch(() => {
        if (detailRequestRef.current !== requestId) return;
        setDetail(null);
        setStatus("error");
      });
    return () => {
      if (detailRequestRef.current === requestId) detailRequestRef.current += 1;
    };
  }, [connectionStatus, documentKey, enabled, reloadToken, taskId]);

  useEffect(() => {
    const requestId = ++revisionsRequestRef.current;
    if (!enabled || !taskId || !documentKey) {
      setRevisions(EMPTY_REVISIONS);
      setRevisionsStatus("idle");
      setHasMoreRevisions(false);
      return;
    }
    setRevisionsStatus("loading");
    listTaskDocumentRevisions(taskId, documentKey, {
      limit: DEFAULT_DOCUMENT_REVISION_PAGE_SIZE,
    })
      .then((page) => {
        if (revisionsRequestRef.current !== requestId) return;
        setRevisions(page.revisions);
        setHasMoreRevisions(page.revisions.length >= page.limit);
        setRevisionsStatus("success");
      })
      .catch(() => {
        if (revisionsRequestRef.current !== requestId) return;
        setRevisions(EMPTY_REVISIONS);
        setHasMoreRevisions(false);
        setRevisionsStatus("error");
      });
    return () => {
      if (revisionsRequestRef.current === requestId) revisionsRequestRef.current += 1;
    };
  }, [connectionStatus, documentKey, enabled, reloadToken, taskId]);

  const loadMoreRevisions = useCallback(() => {
    if (!taskId || !documentKey || revisionsStatus === "loading") return;
    const oldest = revisions[revisions.length - 1];
    if (!oldest) return;
    const requestId = ++revisionsRequestRef.current;
    setRevisionsStatus("loading");
    listTaskDocumentRevisions(taskId, documentKey, {
      limit: DEFAULT_DOCUMENT_REVISION_PAGE_SIZE,
      beforeRevision: oldest.revision_number,
    })
      .then((page) => {
        if (revisionsRequestRef.current !== requestId) return;
        setRevisions((current) => [...current, ...page.revisions]);
        setHasMoreRevisions(page.revisions.length >= page.limit);
        setRevisionsStatus("success");
      })
      .catch(() => {
        if (revisionsRequestRef.current !== requestId) return;
        setRevisionsStatus("error");
      });
  }, [documentKey, revisions, revisionsStatus, taskId]);

  const reload = useCallback(() => setReloadToken((token) => token + 1), []);

  return {
    detail,
    status,
    revisions,
    revisionsStatus,
    hasMoreRevisions,
    loadMoreRevisions,
    reload,
  };
}
