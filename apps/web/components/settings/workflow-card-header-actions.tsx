"use client";

import { IconCopy, IconDownload, IconTrash, IconUpload } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useToast } from "@/components/toast-provider";
import { handleExportWorkflow } from "./workflow-export-actions";

type WorkflowCardHeaderActionsProps = {
  workflowId: string;
  setExportYaml: (json: string) => void;
  setExportOpen: (open: boolean) => void;
  toast: ReturnType<typeof useToast>["toast"];
  onDeleteClick: () => Promise<void>;
  onDuplicateClick: () => Promise<void>;
  deleteDisabled: boolean;
  exportDisabled: boolean;
  duplicateDisabled: boolean;
  duplicateDisabledReason?: string;
  duplicateLoading: boolean;
  readOnly: boolean;
  onUpdateFromFile: () => void;
};

function DeleteWorkflowButton({
  readOnly,
  deleteDisabled,
  onDeleteClick,
  toast,
}: {
  readOnly: boolean;
  deleteDisabled: boolean;
  onDeleteClick: () => Promise<void>;
  toast: ReturnType<typeof useToast>["toast"];
}) {
  const { t } = useTranslation();
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span tabIndex={readOnly ? 0 : undefined} className="inline-flex">
          <Button
            type="button"
            variant="destructive"
            onClick={() => {
              void onDeleteClick().catch((error) => {
                toast({
                  title: t("workflows:failedToDeleteWorkflow"),
                  description: error instanceof Error ? error.message : t("common:requestFailed"),
                  variant: "error",
                });
              });
            }}
            disabled={deleteDisabled}
            className="cursor-pointer"
            data-testid="delete-workflow-button"
          >
            <IconTrash className="h-4 w-4 mr-2" />
            {t("workflows:deleteWorkflow")}
          </Button>
        </span>
      </TooltipTrigger>
      {readOnly && <TooltipContent>{t("workflows:syncedReadOnlyReason")}</TooltipContent>}
    </Tooltip>
  );
}

export function WorkflowCardHeaderActions({
  workflowId,
  setExportYaml,
  setExportOpen,
  toast,
  onDeleteClick,
  onDuplicateClick,
  deleteDisabled,
  exportDisabled,
  duplicateDisabled,
  duplicateDisabledReason,
  duplicateLoading,
  readOnly,
  onUpdateFromFile,
}: WorkflowCardHeaderActionsProps) {
  const { t } = useTranslation();
  const duplicateTooltip =
    duplicateDisabledReason ?? (duplicateLoading ? t("workflows:duplicatingWorkflow") : undefined);
  return (
    <div className="flex flex-wrap justify-end gap-2">
      {!readOnly && !exportDisabled && (
        <Button
          type="button"
          variant="outline"
          onClick={onUpdateFromFile}
          className="cursor-pointer"
          data-testid="update-workflow-from-file"
        >
          <IconUpload className="mr-2 h-4 w-4" />
          {t("workflows:updateFromFile")}
        </Button>
      )}
      <Tooltip>
        <TooltipTrigger asChild>
          <span
            tabIndex={exportDisabled ? 0 : undefined}
            aria-label={exportDisabled ? t("workflows:saveBeforeExporting") : undefined}
          >
            <Button
              type="button"
              variant="outline"
              onClick={() =>
                handleExportWorkflow({ workflowId, setExportYaml, setExportOpen, toast })
              }
              className="cursor-pointer"
              disabled={exportDisabled}
            >
              <IconDownload className="h-4 w-4 mr-2" />
              {t("workflows:export")}
            </Button>
          </span>
        </TooltipTrigger>
        {exportDisabled && <TooltipContent>{t("workflows:saveBeforeExporting")}</TooltipContent>}
      </Tooltip>
      <Tooltip>
        <TooltipTrigger asChild>
          <span
            tabIndex={duplicateDisabled ? 0 : -1}
            className="inline-flex"
            aria-label={duplicateTooltip}
          >
            <Button
              type="button"
              variant="outline"
              onClick={() => void onDuplicateClick()}
              disabled={duplicateDisabled}
              className="cursor-pointer"
              data-testid="duplicate-workflow-button"
            >
              <IconCopy className="h-4 w-4 mr-2" />
              {duplicateLoading
                ? t("workflows:duplicatingWorkflow")
                : t("workflows:duplicateWorkflow")}
            </Button>
          </span>
        </TooltipTrigger>
        {duplicateTooltip && <TooltipContent>{duplicateTooltip}</TooltipContent>}
      </Tooltip>
      <DeleteWorkflowButton
        readOnly={readOnly}
        deleteDisabled={deleteDisabled}
        onDeleteClick={onDeleteClick}
        toast={toast}
      />
    </div>
  );
}
