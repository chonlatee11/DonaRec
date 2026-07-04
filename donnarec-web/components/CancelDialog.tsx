"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { AlertTriangle } from "lucide-react";
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";

type CancelMode = "void" | "reissue";

interface CancelDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  mode: CancelMode;
  /** Receipt number to show in body copy (e.g. "2569/000001") */
  receiptFormatted: string | null;
  /**
   * When true, shows the HIGH-RISK e-Donation keyed warning banner
   * + a mandatory second textarea for RD confirmation reason (D-51).
   */
  eDonationKeyed: boolean;
  /** True while API call is in-flight */
  isSubmitting?: boolean;
  /**
   * Called when user confirms.
   * Returns null on success or { error } string on server rejection.
   * The 409 ErrEDonationKeyedConfirmation case is surfaced via this error.
   */
  onConfirm: (body: {
    reason: string;
    rd_confirmation_reason?: string;
  }) => Promise<{ error?: string } | null>;
}

/**
 * CancelDialog — void / void-and-reissue confirmation AlertDialog.
 *
 * UI-SPEC §"Destructive Confirmation Dialogs":
 *
 *  (a) Void (edonation_keyed=false): single mandatory reason textarea
 *  (b) Void (edonation_keyed=true) HIGH RISK: red-50 warning banner
 *      + TWO mandatory textareas (reason + RD confirmation)
 *  (c) Void & Reissue: body explains new replacement draft + keyed warning
 *      when applicable
 *
 * T-03-36: CancelDialog forces rd_confirmation_reason textarea when keyed;
 *   server re-checks and returns 409 ErrEDonationKeyedConfirmation if missing.
 *
 * Confirm button is blocked until required textareas are non-empty.
 */
export function CancelDialog({
  open,
  onOpenChange,
  mode,
  receiptFormatted,
  eDonationKeyed,
  isSubmitting = false,
  onConfirm,
}: CancelDialogProps) {
  const t = useTranslations();

  const [reason, setReason] = useState("");
  const [rdReason, setRdReason] = useState("");
  const [serverError, setServerError] = useState<string | null>(null);

  const reasonEmpty = reason.trim().length === 0;
  const rdReasonEmpty = eDonationKeyed && rdReason.trim().length === 0;
  const confirmBlocked = reasonEmpty || rdReasonEmpty || isSubmitting;

  const handleConfirm = async () => {
    if (confirmBlocked) return;
    setServerError(null);
    const body = {
      reason: reason.trim(),
      ...(eDonationKeyed ? { rd_confirmation_reason: rdReason.trim() } : {}),
    };
    const result = await onConfirm(body);
    if (result?.error) {
      setServerError(result.error);
      // Keep dialog open so user can correct (409 keyed confirmation case)
    } else {
      // Success — reset and close
      setReason("");
      setRdReason("");
      setServerError(null);
      onOpenChange(false);
    }
  };

  const handleOpenChange = (open: boolean) => {
    if (!open) {
      setReason("");
      setRdReason("");
      setServerError(null);
    }
    onOpenChange(open);
  };

  // ── Title ─────────────────────────────────────────────────────────────────

  const title =
    mode === "reissue"
      ? t("dialogs.voidAndReissue.title")
      : eDonationKeyed
      ? t("dialogs.voidWithEDonation.title")
      : t("dialogs.voidReceipt.title");

  // ── Body copy ─────────────────────────────────────────────────────────────

  const bodyText =
    mode === "reissue"
      ? t("dialogs.voidAndReissue.body", {
          receiptFormatted: receiptFormatted ?? "",
        })
      : t("dialogs.voidReceipt.body", {
          receiptFormatted: receiptFormatted ?? "",
        });

  // ── Confirm button label ──────────────────────────────────────────────────

  const confirmLabel =
    mode === "reissue"
      ? t("dialogs.voidAndReissue.confirm")
      : eDonationKeyed
      ? t("dialogs.voidWithEDonation.confirm")
      : t("dialogs.voidReceipt.confirm");

  // ── Reason textarea label ─────────────────────────────────────────────────

  const reasonLabel =
    mode === "reissue"
      ? t("dialogs.voidAndReissue.reasonLabel")
      : t("dialogs.voidReceipt.reasonLabel");

  return (
    <AlertDialog open={open} onOpenChange={handleOpenChange}>
      <AlertDialogContent className="max-w-lg">
        <AlertDialogHeader>
          <AlertDialogTitle>{title}</AlertDialogTitle>
          <AlertDialogDescription asChild>
            <div className="flex flex-col gap-3 text-[14px] text-slate-700">
              <p>{bodyText}</p>

              {/* e-Donation keyed HIGH-RISK warning banner */}
              {eDonationKeyed && (
                <div
                  className="flex items-start gap-2.5 rounded-md border border-red-200 bg-red-50 px-4 py-3"
                  role="alert"
                >
                  <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-red-600" />
                  <p className="text-[13px] leading-relaxed text-red-700">
                    {t("dialogs.voidWithEDonation.alert")}
                  </p>
                </div>
              )}
            </div>
          </AlertDialogDescription>
        </AlertDialogHeader>

        {/* ── Reason textarea ────────────────────────────────────────────── */}
        <div className="flex flex-col gap-4 py-1">
          <div className="flex flex-col gap-1.5">
            <Label
              htmlFor="cancel-reason"
              className="text-[14px] font-medium text-slate-700"
            >
              {reasonLabel}
            </Label>
            <Textarea
              id="cancel-reason"
              rows={3}
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              aria-required="true"
              aria-invalid={reasonEmpty ? "true" : "false"}
              className="resize-y"
              placeholder="ระบุเหตุผล..."
              disabled={isSubmitting}
            />
            {reasonEmpty && reason !== "" && (
              <p className="text-[13px] text-red-600" role="alert">
                {t("errors.cancelWithoutReason")}
              </p>
            )}
          </div>

          {/* Second textarea — RD confirmation (only when e-Donation keyed) */}
          {eDonationKeyed && (
            <div className="flex flex-col gap-1.5">
              <Label
                htmlFor="rd-confirm-reason"
                className="text-[14px] font-medium text-slate-700"
              >
                {t("dialogs.voidWithEDonation.rdConfirmLabel")}
              </Label>
              <Textarea
                id="rd-confirm-reason"
                rows={3}
                value={rdReason}
                onChange={(e) => setRdReason(e.target.value)}
                aria-required="true"
                aria-invalid={rdReasonEmpty ? "true" : "false"}
                className="resize-y"
                placeholder="ระบุการดำเนินการฝั่ง RD..."
                disabled={isSubmitting}
              />
              {rdReasonEmpty && rdReason !== "" && (
                <p className="text-[13px] text-red-600" role="alert">
                  {t("errors.reasonRequired")}
                </p>
              )}
            </div>
          )}

          {/* Server error (e.g. 409 ErrEDonationKeyedConfirmation) */}
          {serverError && (
            <p
              className="text-[13px] text-red-600"
              role="alert"
              aria-live="assertive"
            >
              {serverError}
            </p>
          )}
        </div>

        <AlertDialogFooter>
          <AlertDialogCancel disabled={isSubmitting}>
            {t("actions.cancel")}
          </AlertDialogCancel>
          <Button
            type="button"
            variant="destructive"
            disabled={confirmBlocked}
            onClick={handleConfirm}
            aria-disabled={confirmBlocked}
          >
            {isSubmitting ? "กำลังดำเนินการ..." : confirmLabel}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
