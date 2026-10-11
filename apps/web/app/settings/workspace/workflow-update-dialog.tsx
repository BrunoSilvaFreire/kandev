"use client";

import { useState } from "react";
import { parse } from "yaml";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@kandev/ui/dialog";
import { Label } from "@kandev/ui/label";
import { useToast } from "@/components/toast-provider";
import { applyWorkflowUpdateAction, previewWorkflowUpdateAction } from "@/app/actions/workspaces";
import type { Workflow, WorkflowUpdatePreview, Workspace } from "@/lib/types/http";

type Props = {
  workspace: Workspace;
  workflow: Workflow;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onUpdated: () => void;
};

function WorkflowUpdateDiff({ preview }: { preview: WorkflowUpdatePreview }) {
  const { t } = useTranslation();
  const changeLabels: Record<string, string> = {
    added: t("workflows:updateChange_added"),
    modified: t("workflows:updateChange_modified"),
    removed: t("workflows:updateChange_removed"),
    blocked: t("workflows:updateChange_blocked"),
  };
  return (
    <section
      aria-label={t("workflows:updateDiffTitle")}
      className="space-y-2 rounded-md border p-3"
    >
      <h3 className="font-medium">{t("workflows:updateDiffTitle")}</h3>
      {!preview.changes.length && (
        <p className="text-sm text-muted-foreground">{t("workflows:updateNoChanges")}</p>
      )}
      <ul className="space-y-2">
        {preview.changes.map((change, i) => (
          <li key={`${change.path}:${i}`} className="rounded border p-2 text-sm">
            <div className="font-medium">
              {changeLabels[change.kind] ?? change.kind}: {change.path}
            </div>
            {change.before !== undefined && (
              <pre className="mt-1 whitespace-pre-wrap break-words text-xs text-muted-foreground">
                {t("workflows:updateBefore")}: {JSON.stringify(change.before, null, 2)}
              </pre>
            )}
            {change.after !== undefined && (
              <pre className="mt-1 whitespace-pre-wrap break-words text-xs">
                {t("workflows:updateAfter")}: {JSON.stringify(change.after, null, 2)}
              </pre>
            )}
          </li>
        ))}
      </ul>
    </section>
  );
}

function parseWorkflowCount(yaml: string): number {
  if (!yaml) return 1;
  try {
    const parsed = parse(yaml) as { workflows?: unknown[] };
    return Math.max(1, parsed?.workflows?.length ?? 1);
  } catch {
    return 1;
  }
}

type ActionsProps = {
  preview: WorkflowUpdatePreview | null;
  hasDefinition: boolean;
  index: number | null;
  workflowCount: number;
  busy: boolean;
  onCancel: () => void;
  onPreview: () => void;
  onChooseAnother: () => void;
  onApply: () => void;
};

function WorkflowUpdateActions({
  preview,
  hasDefinition,
  index,
  workflowCount,
  busy,
  onCancel,
  onPreview,
  onChooseAnother,
  onApply,
}: ActionsProps) {
  const { t } = useTranslation();
  const needsChoice = workflowCount > 1 && index === null;
  return (
    <DialogFooter className="shrink-0">
      <Button variant="outline" onClick={onCancel} disabled={busy}>
        {t("common:cancel")}
      </Button>
      {!preview && (
        <Button onClick={onPreview} disabled={!hasDefinition || needsChoice || busy}>
          {busy ? t("workflows:updatePreviewing") : t("workflows:updatePreview")}
        </Button>
      )}
      {preview && (
        <>
          <Button variant="outline" onClick={onChooseAnother} disabled={busy}>
            {t("workflows:updateChooseAnotherFile")}
          </Button>
          <Button onClick={onApply} disabled={!preview.can_apply || busy}>
            {busy ? t("workflows:updateApplying") : t("workflows:updateConfirm")}
          </Button>
        </>
      )}
    </DialogFooter>
  );
}

function useWorkflowUpdateDialog(
  workspace: Workspace,
  workflow: Workflow,
  onOpenChange: (open: boolean) => void,
  onUpdated: () => void,
) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const [yaml, setYaml] = useState("");
  const [index, setIndex] = useState<number | null>(null);
  const [preview, setPreview] = useState<WorkflowUpdatePreview | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const reset = () => {
    setYaml("");
    setPreview(null);
    setIndex(null);
    setError("");
    setBusy(false);
  };
  const close = (next: boolean) => {
    if (busy) return;
    if (!next) reset();
    onOpenChange(next);
  };
  const fileChanged = async (file?: File) => {
    reset();
    if (!file) return;
    try {
      const content = await file.text();
      const parsed = parse(content) as { workflows?: unknown[] };
      if (!Array.isArray(parsed?.workflows) || parsed.workflows.length === 0)
        throw new Error(t("workflows:updateInvalidFile"));
      setYaml(content);
      setIndex(parsed.workflows.length === 1 ? 0 : null);
      if (parsed.workflows.length > 1)
        setError(t("workflows:updateChooseWorkflow", { count: parsed.workflows.length }));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : t("workflows:updateInvalidFile"));
    }
  };
  const selectedIndex = (value: string) => {
    setIndex(value === "" ? null : Number(value));
    setPreview(null);
    setError("");
  };
  const showPreview = async () => {
    if (!yaml.trim()) return;
    setBusy(true);
    setError("");
    try {
      const response = await previewWorkflowUpdateAction(
        workspace.id,
        workflow.id,
        yaml,
        index ?? 0,
      );
      setPreview(response);
      if (response.errors.length) setError(response.errors.join("\n"));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : t("common:requestFailed"));
    } finally {
      setBusy(false);
    }
  };
  const apply = async () => {
    if (index === null || !preview?.can_apply || !preview.version) return;
    setBusy(true);
    try {
      await applyWorkflowUpdateAction(workspace.id, workflow.id, yaml, preview.version, index);
      toast({ title: t("workflows:updateComplete"), variant: "success" });
      reset();
      onOpenChange(false);
      onUpdated();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : t("common:requestFailed"));
    } finally {
      setBusy(false);
    }
  };
  const chooseAnother = () => {
    setPreview(null);
    setError("");
  };

  return {
    t,
    yaml,
    index,
    preview,
    busy,
    error,
    workflowCount: parseWorkflowCount(yaml),
    close,
    fileChanged,
    selectedIndex,
    showPreview,
    apply,
    chooseAnother,
  };
}

export function WorkflowUpdateDialog({
  workspace,
  workflow,
  open,
  onOpenChange,
  onUpdated,
}: Props) {
  const {
    t,
    yaml,
    index,
    preview,
    busy,
    error,
    workflowCount,
    close,
    fileChanged,
    selectedIndex,
    showPreview,
    apply,
    chooseAnother,
  } = useWorkflowUpdateDialog(workspace, workflow, onOpenChange, onUpdated);

  return (
    <Dialog open={open} onOpenChange={close}>
      <DialogContent className="flex max-h-[90dvh] flex-col overflow-hidden sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>{t("workflows:updateFromFileTitle", { name: workflow.name })}</DialogTitle>
        </DialogHeader>
        <div className="min-h-0 space-y-4 overflow-y-auto">
          <div className="space-y-2">
            <Label htmlFor="workflow-update-file">{t("workflows:updateChooseFile")}</Label>
            <input
              id="workflow-update-file"
              type="file"
              accept=".yml,.yaml"
              disabled={busy}
              onChange={(event) => void fileChanged(event.target.files?.[0])}
              className="block w-full min-w-0 text-sm file:mr-3 file:h-11 file:rounded-md file:border file:border-input file:bg-muted file:px-3 file:text-foreground"
            />
          </div>
          {workflowCount > 1 && (
            <div className="space-y-2">
              <Label htmlFor="workflow-update-index">{t("workflows:updateSelectDefinition")}</Label>
              <select
                id="workflow-update-index"
                className="h-11 w-full rounded-md border bg-background px-3"
                value={index ?? ""}
                onChange={(event) => selectedIndex(event.target.value)}
              >
                <option value="">{t("workflows:updateSelectDefinition")}</option>
                {Array.from({ length: workflowCount }, (_, i) => (
                  <option key={i} value={i}>
                    {t("workflows:updateDefinitionNumber", { number: i + 1 })}
                  </option>
                ))}
              </select>
            </div>
          )}
          {error && (
            <p
              role="alert"
              className="whitespace-pre-wrap rounded-md border border-destructive/40 bg-destructive/5 p-3 text-sm text-destructive"
            >
              {error}
            </p>
          )}
          {preview && <WorkflowUpdateDiff preview={preview} />}
        </div>
        <WorkflowUpdateActions
          preview={preview}
          hasDefinition={Boolean(yaml.trim())}
          index={index}
          workflowCount={workflowCount}
          busy={busy}
          onCancel={() => close(false)}
          onPreview={() => void showPreview()}
          onChooseAnother={chooseAnother}
          onApply={() => void apply()}
        />
      </DialogContent>
    </Dialog>
  );
}
