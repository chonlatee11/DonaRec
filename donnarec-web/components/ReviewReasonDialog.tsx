"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";

export interface ReviewReasonDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /**
   * "return"  → reversible action → shadcn Dialog (not AlertDialog).
   * "reject"  → terminal action  → shadcn AlertDialog (focus-trapped, destructive).
   *
   * UI-SPEC §Destructive Confirmation Dialogs:
   *   ตีกลับแก้ไข uses Dialog; ปฏิเสธถาวร uses AlertDialog.
   */
  variant: "return" | "reject";
  /**
   * Called with the trimmed reason string when the user confirms.
   * Must be non-empty; the dialog blocks confirm until a reason is typed.
   */
  onConfirm: (reason: string) => Promise<void>;
  /** Whether a mutation is in flight — disables buttons during submit */
  isSubmitting?: boolean;
}

/**
 * ReviewReasonDialog — reusable dialog for return-for-edit and permanent-reject.
 *
 * Acceptance criteria:
 * - Blocks confirm until reason textarea is non-empty (errors.reasonRequired)
 * - "return" variant uses Dialog (reversible, outline confirm button)
 * - "reject" variant uses AlertDialog (terminal, destructive confirm button)
 * - Focus returns to trigger on close (AlertDialog handles this natively)
 */
export function ReviewReasonDialog({
  open,
  onOpenChange,
  variant,
  onConfirm,
  isSubmitting = false,
}: ReviewReasonDialogProps) {
  const t = useTranslations();

  const [reason, setReason] = useState("");
  const [showError, setShowError] = useState(false);

  // Resolve i18n keys from dialog variant
  const dialogKey = variant === "return" ? "returnForEdit" : "rejectPermanent";

  async function handleConfirm() {
    if (!reason.trim()) {
      setShowError(true);
      return;
    }
    setShowError(false);
    await onConfirm(reason.trim());
    // Parent controls closing via onOpenChange; reset local state here
    setReason("");
  }

  function handleClose(nextOpen: boolean) {
    if (!nextOpen) {
      setReason("");
      setShowError(false);
    }
    onOpenChange(nextOpen);
  }

  // ── Shared form content ────────────────────────────────────────────────────

  const reasonField = (
    <div className="flex flex-col gap-1.5">
      <Label htmlFor="review-reason" className="text-[14px]">
        {t(`dialogs.${dialogKey}.reasonLabel`)}
      </Label>
      <Textarea
        id="review-reason"
        value={reason}
        onChange={(e) => {
          setReason(e.target.value);
          if (e.target.value.trim()) setShowError(false);
        }}
        rows={3}
        className={showError ? "border-red-500 focus-visible:ring-red-500" : ""}
        aria-required="true"
        aria-describedby={showError ? "reason-error" : undefined}
        disabled={isSubmitting}
      />
      {showError && (
        <p
          id="reason-error"
          className="text-[14px] text-red-600"
          role="alert"
        >
          {t("errors.reasonRequired")}
        </p>
      )}
    </div>
  );

  const cancelLabel = t("actions.cancel");
  const confirmLabel = t(`dialogs.${dialogKey}.confirm`);
  const submitLabel = isSubmitting ? "กำลังดำเนินการ..." : confirmLabel;

  // ── Return variant: reversible → Dialog ───────────────────────────────────

  if (variant === "return") {
    return (
      <Dialog open={open} onOpenChange={handleClose}>
        <DialogContent className="max-w-[480px]">
          <DialogHeader>
            <DialogTitle>{t(`dialogs.${dialogKey}.title`)}</DialogTitle>
            <DialogDescription>
              {t(`dialogs.${dialogKey}.body`)}
            </DialogDescription>
          </DialogHeader>
          <div className="py-2">{reasonField}</div>
          <DialogFooter className="gap-2 sm:gap-2">
            <Button
              variant="ghost"
              onClick={() => handleClose(false)}
              disabled={isSubmitting}
            >
              {cancelLabel}
            </Button>
            {/* UI-SPEC: "ตีกลับแก้ไข" uses outline variant (not destructive) */}
            <Button
              variant="outline"
              onClick={handleConfirm}
              disabled={isSubmitting || !reason.trim()}
            >
              {submitLabel}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    );
  }

  // ── Reject variant: terminal → AlertDialog ────────────────────────────────

  return (
    <AlertDialog open={open} onOpenChange={handleClose}>
      <AlertDialogContent className="max-w-[480px]">
        <AlertDialogHeader>
          <AlertDialogTitle>{t(`dialogs.${dialogKey}.title`)}</AlertDialogTitle>
          <AlertDialogDescription>
            {t(`dialogs.${dialogKey}.body`)}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <div className="py-2">{reasonField}</div>
        <AlertDialogFooter className="gap-2 sm:gap-2">
          <Button
            variant="outline"
            onClick={() => handleClose(false)}
            disabled={isSubmitting}
          >
            {cancelLabel}
          </Button>
          {/* UI-SPEC: "ปฏิเสธถาวร" uses destructive variant */}
          <Button
            variant="destructive"
            onClick={handleConfirm}
            disabled={isSubmitting || !reason.trim()}
          >
            {submitLabel}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
