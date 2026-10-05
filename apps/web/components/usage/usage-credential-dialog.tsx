"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { Label } from "@kandev/ui/label";
import { Textarea } from "@kandev/ui/textarea";
import { useToast } from "@/components/toast-provider";
import type { ProviderUsageCredentialHint } from "@/lib/types/provider-usage";
import { saveUsageCredential } from "@/lib/usage/save-usage-credential";

/** Per-kind instructions for obtaining the credential value. */
export function credentialInstructionsKey(kind: ProviderUsageCredentialHint["kind"]): string {
  if (kind === "opencode_console_cookie") return "usage:credentialOpencodeCookieInstructions";
  return "usage:credentialJunieApiKeyInstructions";
}

export function UsageCredentialButton({
  hint,
  onSaved,
}: {
  hint: ProviderUsageCredentialHint;
  onSaved?: () => void;
}) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const [open, setOpen] = useState(false);
  const [value, setValue] = useState("");
  const [saving, setSaving] = useState(false);

  const handleSave = async () => {
    setSaving(true);
    try {
      await saveUsageCredential(hint, value);
      toast({ title: t("usage:credentialSaved"), variant: "success" });
      setOpen(false);
      setValue("");
      onSaved?.();
    } catch {
      toast({ title: t("usage:credentialSaveFailed"), variant: "error" });
    } finally {
      setSaving(false);
    }
  };

  return (
    <>
      <Button
        variant="outline"
        size="sm"
        onClick={() => setOpen(true)}
        className="min-h-8 cursor-pointer [@media(pointer:coarse)]:min-h-11"
        data-testid="usage-credential-button"
      >
        {hint.configured ? t("usage:credentialUpdate") : t("usage:credentialSet")}
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("usage:credentialDialogTitle")}</DialogTitle>
            <DialogDescription>{t(credentialInstructionsKey(hint.kind))}</DialogDescription>
          </DialogHeader>
          <div className="space-y-2">
            <Label htmlFor="usage-credential-value">{t("usage:credentialValueLabel")}</Label>
            <Textarea
              id="usage-credential-value"
              value={value}
              onChange={(event) => setValue(event.target.value)}
              autoComplete="off"
              spellCheck={false}
              className="font-mono text-xs"
              data-testid="usage-credential-input"
            />
          </div>
          <DialogFooter>
            <Button
              onClick={handleSave}
              disabled={saving || value.trim().length === 0}
              className="cursor-pointer"
              data-testid="usage-credential-save"
            >
              {t("usage:credentialSave")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
