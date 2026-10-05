"use client";

import { useTranslation } from "react-i18next";
import {
  IconChartBar,
  IconFileDescription,
  IconHistory,
  IconLayoutGrid,
  IconTimeline,
} from "@tabler/icons-react";
import type { Canvas } from "@/lib/api/domains/canvas-api";
import { pluginPanelId } from "@/lib/state/layout-manager/plugin-panels";
import type { MobileSessionPanel } from "@/lib/state/slices/ui/types";
import { resolvePluginIcon } from "@/lib/plugins/icons";
import { pluginRegistry, usePluginRegistry } from "@/lib/plugins/registry";
import { registrationIsVisible } from "../plugin-task-panel";
import { resolveTaskPanelTitle } from "@/lib/state/layout-manager/plugin-panels";
import { MobilePickerSheet } from "./mobile-picker-sheet";

type PluginPanelPickerProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSelect: (panel: MobileSessionPanel) => void;
  showPromptHistory?: boolean;
  showUsage?: boolean;
  showDocuments?: boolean;
  showTaskHistory?: boolean;
  taskCanvases?: Canvas[];
  onOpenCanvas?: (canvasId: string) => void;
  taskId?: string | null;
  sessionId?: string | null;
  sessionKind?: "managed" | "passthrough" | null;
};

const PICKER_OPTION_CLASS =
  "flex min-h-11 w-full min-w-0 cursor-pointer items-center gap-3 rounded-md px-3 py-2 text-left text-sm hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring";

/** A single 44px picker row with a leading icon and label. */
function PickerOption({
  testId,
  icon,
  label,
  onSelect,
}: {
  testId: string;
  icon: React.ReactNode;
  label: string;
  onSelect: () => void;
}) {
  return (
    <button type="button" data-testid={testId} className={PICKER_OPTION_CLASS} onClick={onSelect}>
      {icon}
      <span className="min-w-0 truncate">{label}</span>
    </button>
  );
}

/** Core (non-plugin) picker rows: Prompt history, Usage, Documents. */
function CorePanelOptions({
  showPromptHistory,
  showUsage,
  showDocuments,
  showTaskHistory,
  onSelect,
}: {
  showPromptHistory: boolean;
  showUsage: boolean;
  showDocuments: boolean;
  showTaskHistory: boolean;
  onSelect: (panel: MobileSessionPanel) => void;
}) {
  const { t } = useTranslation();
  return (
    <>
      {showPromptHistory && (
        <PickerOption
          testId="mobile-prompt-history-option"
          icon={<IconHistory className="h-5 w-5 shrink-0 text-muted-foreground" />}
          label={t("task:promptHistory")}
          onSelect={() => onSelect("prompt-history")}
        />
      )}
      {showUsage && (
        <PickerOption
          testId="mobile-usage-option"
          icon={<IconChartBar className="h-5 w-5 shrink-0 text-muted-foreground" />}
          label={t("task:panelUsage")}
          onSelect={() => onSelect("usage")}
        />
      )}
      {showDocuments && (
        <PickerOption
          testId="mobile-documents-option"
          icon={<IconFileDescription className="h-5 w-5 shrink-0 text-muted-foreground" />}
          label={t("task:panelDocuments")}
          onSelect={() => onSelect("documents")}
        />
      )}
      {showTaskHistory && (
        <PickerOption
          testId="mobile-task-history-option"
          icon={<IconTimeline className="h-5 w-5 shrink-0 text-muted-foreground" />}
          label={t("task:panelTaskHistory")}
          onSelect={() => onSelect("task-history")}
        />
      )}
    </>
  );
}

/** One grouped, scrollable phone picker for all mobile-enabled plugin panels. */
export function PluginPanelPicker({
  open,
  onOpenChange,
  onSelect,
  showPromptHistory = false,
  showUsage = false,
  showDocuments = false,
  showTaskHistory = false,
  taskCanvases = [],
  onOpenCanvas,
  taskId = null,
  sessionId = null,
  sessionKind = null,
}: PluginPanelPickerProps) {
  const { t } = useTranslation();
  usePluginRegistry();
  const registrations = taskId
    ? pluginRegistry
        .getTaskPanels()
        .filter((registration) => registration.mobileEnabled)
        .filter((registration) =>
          registrationIsVisible(registration, {
            taskId,
            sessionId,
            sessionKind,
            presentation: "mobile",
          }),
        )
    : [];

  if (!open) return null;

  return (
    <MobilePickerSheet open={open} onOpenChange={onOpenChange} title={t("common:panels")}>
      <div className="space-y-1" data-testid="mobile-plugin-panel-options">
        {taskCanvases.map((canvas) => (
          <button
            key={canvas.id}
            type="button"
            data-testid={`mobile-canvas-option-${canvas.id}`}
            className={PICKER_OPTION_CLASS}
            onClick={() => {
              onOpenCanvas?.(canvas.id);
              onOpenChange(false);
            }}
          >
            <IconLayoutGrid className="h-5 w-5 shrink-0 text-muted-foreground" aria-hidden="true" />
            <span className="min-w-0 truncate">{canvas.title}</span>
          </button>
        ))}
        <CorePanelOptions
          showPromptHistory={showPromptHistory}
          showUsage={showUsage}
          showDocuments={showDocuments}
          showTaskHistory={showTaskHistory}
          onSelect={(panel) => {
            onSelect(panel);
            onOpenChange(false);
          }}
        />
        {registrations.map((registration) => {
          const panelId = pluginPanelId(registration.pluginId, registration.id);
          const Icon = resolvePluginIcon(registration.icon);
          return (
            <button
              key={panelId}
              type="button"
              data-testid={`mobile-plugin-panel-option-${registration.pluginId}-${registration.id}`}
              data-panel-id={panelId}
              className={PICKER_OPTION_CLASS}
              onClick={() => {
                onSelect(panelId as MobileSessionPanel);
                onOpenChange(false);
              }}
            >
              <Icon className="h-5 w-5 shrink-0 text-muted-foreground" />
              <span className="min-w-0 truncate">{resolveTaskPanelTitle(registration)}</span>
            </button>
          );
        })}
      </div>
    </MobilePickerSheet>
  );
}
