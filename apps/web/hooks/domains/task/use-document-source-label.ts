"use client";

import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";

const EMPTY = "";

/**
 * Resolve a persisted document/revision provenance pair to a display label.
 * `sessionName` and `stepName` are read from live store data when available;
 * when neither source is known the label is null so callers can render an
 * explicit Unknown/legacy fallback instead of guessing.
 */
export function useDocumentSourceLabel(
  sessionId: string | null | undefined,
  stepId: string | null | undefined,
): string | null {
  const { t } = useTranslation();
  const sessionName = useAppStore((state) =>
    sessionId ? (state.taskSessions.items[sessionId]?.name ?? EMPTY) : EMPTY,
  );
  const stepName = useAppStore((state) => {
    if (!stepId) return EMPTY;
    const active = state.kanban.steps.find((step) => step.id === stepId);
    if (active) return active.title;
    for (const snapshot of Object.values(state.kanbanMulti.snapshots)) {
      const match = snapshot?.steps.find((step) => step.id === stepId);
      if (match) return match.title;
    }
    return EMPTY;
  });

  return useMemo(() => {
    const parts: string[] = [];
    if (sessionName || sessionId) {
      parts.push(t("task:sourceSession", { name: sessionName || sessionId }));
    }
    if (stepName || stepId) {
      parts.push(t("task:sourceStep", { name: stepName || stepId }));
    }
    return parts.length > 0 ? parts.join(" · ") : null;
  }, [sessionId, sessionName, stepId, stepName, t]);
}
