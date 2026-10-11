"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { IconDots } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@kandev/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { Input } from "@kandev/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import type { KanbanViewPresets } from "@/hooks/use-kanban-view-presets";
import type { ViewPresetNameError } from "@/lib/view-model/presets";

// Catalog keys, not copy: module scope must not resolve `t()`.
const NAME_ERROR_KEYS: Record<ViewPresetNameError, string> = {
  empty: "task:presetErrorEmpty",
  too_long: "task:presetErrorTooLong",
  duplicate: "task:presetErrorDuplicate",
  limit: "task:presetErrorLimit",
};

// Select needs a value for "no preset active"; it is never stored.
const NO_PRESET = "__none__";

type NameDialogState = { mode: "create" } | { mode: "rename"; id: string; name: string } | null;

function PresetNameDialog({
  state,
  presets,
  onClose,
}: {
  state: Exclude<NameDialogState, null>;
  presets: KanbanViewPresets;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const [name, setName] = useState(state.mode === "rename" ? state.name : "");
  const [error, setError] = useState<ViewPresetNameError | null>(null);
  const submit = () => {
    const failure = state.mode === "rename" ? presets.rename(state.id, name) : presets.create(name);
    if (failure) setError(failure);
    else onClose();
  };
  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent data-testid="view-preset-name-dialog">
        <DialogHeader>
          <DialogTitle>
            {state.mode === "rename" ? t("task:presetRenameTitle") : t("task:presetCreateTitle")}
          </DialogTitle>
        </DialogHeader>
        <Input
          autoFocus
          value={name}
          aria-label={t("task:presetNameLabel")}
          aria-invalid={error !== null}
          placeholder={t("task:presetNamePlaceholder")}
          onChange={(event) => {
            setName(event.target.value);
            setError(null);
          }}
          data-testid="view-preset-name-input"
        />
        {error && (
          <p role="alert" className="text-xs text-destructive" data-testid="view-preset-name-error">
            {t(NAME_ERROR_KEYS[error])}
          </p>
        )}
        <DialogFooter>
          <Button variant="outline" className="cursor-pointer" onClick={onClose}>
            {t("common:cancel")}
          </Button>
          <Button className="cursor-pointer" onClick={submit} data-testid="view-preset-name-save">
            {t("common:save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/**
 * Named filter and grouping presets for one Home view: pick one to apply it
 * at once, and save, update, rename, or delete explicitly from the actions
 * menu. The badge shows when the live view differs from the active preset.
 */
export function ViewPresetsControl({
  presets,
  testId,
}: {
  presets: KanbanViewPresets;
  testId: string;
}) {
  const { t } = useTranslation();
  const [dialog, setDialog] = useState<NameDialogState>(null);
  const active = presets.active;
  return (
    <div className="flex items-center gap-1" data-testid={testId}>
      <Select
        // A modified view holds no selection, so picking the active preset
        // again re-applies it; the trigger still names it.
        value={presets.dirty ? "" : (active?.id ?? NO_PRESET)}
        onValueChange={(id) => id !== NO_PRESET && presets.apply(id)}
      >
        <SelectTrigger
          className="min-w-28 text-xs max-sm:min-h-11 max-sm:flex-1"
          aria-label={t("task:presetLabel")}
          data-testid={`${testId}-select`}
        >
          <SelectValue placeholder={active?.name} />
        </SelectTrigger>
        <SelectContent>
          {!active && (
            <SelectItem value={NO_PRESET} className="text-xs" disabled>
              {t("task:presetNone")}
            </SelectItem>
          )}
          {presets.items.map((preset) => (
            <SelectItem key={preset.id} value={preset.id} className="text-xs">
              {preset.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      {presets.dirty && (
        <Badge variant="outline" className="shrink-0" data-testid={`${testId}-dirty`}>
          {t("task:presetModified")}
        </Badge>
      )}
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            variant="ghost"
            size="icon"
            className="shrink-0 cursor-pointer max-sm:min-h-11 max-sm:min-w-11"
            aria-label={t("task:presetActions")}
            data-testid={`${testId}-actions`}
          >
            <IconDots className="h-4 w-4" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem
            className="cursor-pointer"
            onSelect={() => setDialog({ mode: "create" })}
            data-testid={`${testId}-save-as`}
          >
            {t("task:presetSaveAs")}
          </DropdownMenuItem>
          <DropdownMenuItem
            className="cursor-pointer"
            disabled={!presets.dirty}
            onSelect={() => presets.saveActive()}
            data-testid={`${testId}-update`}
          >
            {t("task:presetUpdate")}
          </DropdownMenuItem>
          <DropdownMenuItem
            className="cursor-pointer"
            disabled={!active}
            onSelect={() =>
              active && setDialog({ mode: "rename", id: active.id, name: active.name })
            }
            data-testid={`${testId}-rename`}
          >
            {t("common:rename")}
          </DropdownMenuItem>
          <DropdownMenuItem
            className="cursor-pointer"
            disabled={!active}
            onSelect={() => active && presets.remove(active.id)}
            data-testid={`${testId}-delete`}
          >
            {t("common:delete")}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      {dialog && (
        <PresetNameDialog state={dialog} presets={presets} onClose={() => setDialog(null)} />
      )}
    </div>
  );
}
