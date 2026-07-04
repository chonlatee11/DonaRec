"use client";

import { useTranslations } from "next-intl";
import { Download, Mail, Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  DeliveryStatusBadge,
  type EmailDeliveryStatus,
} from "@/components/DeliveryStatusBadge";
import type { EmailDeliveryInfo } from "@/lib/donations";

interface EmailDeliveryPanelProps {
  /** Only rendered by the caller for status issued|cancelled (UI-SPEC Screen 3b) */
  status: "issued" | "cancelled";
  /** Latest send attempt, or null if the worker (04-05) has not processed the job yet */
  emailDelivery: EmailDeliveryInfo | null;
  /**
   * True once the outbox worker has frozen the receipt PDF
   * (donation.receipt_pdf_object_key !== null) — gates the download button's
   * enabled state (D-56).
   */
  hasPdf: boolean;
  /**
   * Checker/Admin only (D-57 "resend gated to Checker/Admin"). Reuses the
   * server-computed `can_reveal_pii` flag (also Checker/Admin-only, T-03-32) —
   * no new server-side flag was added for this since the role gate is
   * identical; Go independently re-enforces RBAC on the actual POST /resend
   * call regardless of what this flag says (T-04-15).
   */
  canResend: boolean;
  onDownload: () => void;
  onResend: () => void;
  isDownloading: boolean;
  isResending: boolean;
  locale?: "th" | "en";
  /** th-TH-u-ca-buddhist formatter, matching DonationDetailView's formatDateTime */
  formatDateTime: (iso: string) => string;
}

/**
 * EmailDeliveryPanel — Screen 3b: status badge + recipient/timestamp/attempts +
 * resend/download buttons + failed-state error copy.
 *
 * Presentational only (no useMutation here) — mirrors the established pattern
 * of Cancel/Reissue wiring living in DonationDetailView while CancelDialog
 * stays presentational (03-13 decision). Rendered by the caller only when
 * donation.status is "issued" or "cancelled".
 */
export function EmailDeliveryPanel({
  status,
  emailDelivery,
  hasPdf,
  canResend,
  onDownload,
  onResend,
  isDownloading,
  isResending,
  locale = "th",
  formatDateTime,
}: EmailDeliveryPanelProps) {
  const t = useTranslations("emailDelivery");

  const badgeStatus: EmailDeliveryStatus = emailDelivery
    ? (emailDelivery.status === "sent"
        ? "sent"
        : emailDelivery.status === "no_email"
        ? "no_email"
        : "failed")
    : "pending";

  // Resend is never shown for cancelled receipts (UI-SPEC: "a cancelled receipt
  // should not be re-sent") and never shown when the donor has no email on file.
  const showResend =
    status !== "cancelled" && canResend && badgeStatus !== "no_email";

  return (
    <div className="rounded-lg border border-slate-200 bg-white p-4">
      <h2 className="mb-3 text-[14px] font-medium text-slate-700">
        {t("heading")}
      </h2>

      <div className="mb-3">
        <DeliveryStatusBadge status={badgeStatus} locale={locale} />
      </div>

      {badgeStatus === "no_email" ? (
        <div className="mb-3">
          <p className="text-[14px] font-medium text-slate-700">
            {t("noEmailHeading")}
          </p>
          <p className="text-[14px] text-slate-500">{t("noEmailBody")}</p>
        </div>
      ) : emailDelivery ? (
        <div className="mb-3 flex flex-col gap-1">
          {emailDelivery.sent_to && (
            <p className="text-[16px] text-slate-900">
              {t("recipientLabel")}: {emailDelivery.sent_to}
            </p>
          )}
          <p className="text-[14px] text-slate-600">
            {t("lastSentLabel")}: {formatDateTime(emailDelivery.last_attempt_at)} ·{" "}
            {t("attemptLabel", { n: emailDelivery.attempts })}
          </p>
          {badgeStatus === "failed" && (
            <p className="text-[14px] text-red-600">
              {t("sendFailed", {
                errorSuffix: emailDelivery.last_error
                  ? ` — ${emailDelivery.last_error}.`
                  : ".",
              })}
            </p>
          )}
          {emailDelivery.provider_message_id && (
            <details className="mt-1">
              <summary className="cursor-pointer text-[14px] text-slate-500">
                {t("providerMsgIdLabel")}
              </summary>
              <p className="mt-1 font-mono text-[14px] text-slate-500">
                {emailDelivery.provider_message_id}
              </p>
            </details>
          )}
        </div>
      ) : (
        <div className="mb-3">
          <p className="text-[14px] font-medium text-slate-700">
            {t("notProcessedHeading")}
          </p>
          <p className="text-[14px] text-slate-500">{t("notProcessedBody")}</p>
        </div>
      )}

      <div className="flex flex-wrap gap-2">
        <Button
          type="button"
          variant="outline"
          className="min-h-[44px]"
          disabled={!hasPdf || isDownloading}
          title={!hasPdf ? t("downloadDisabledTooltip") : undefined}
          onClick={onDownload}
        >
          {isDownloading ? (
            <Loader2 className="mr-1.5 h-4 w-4 animate-spin" />
          ) : (
            <Download className="mr-1.5 h-4 w-4" />
          )}
          {t("download")}
        </Button>

        {showResend && (
          <Button
            type="button"
            variant="outline"
            className="min-h-[44px]"
            disabled={isResending}
            aria-busy={isResending}
            aria-live="polite"
            onClick={onResend}
          >
            {isResending ? (
              <>
                <Loader2 className="mr-1.5 h-4 w-4 animate-spin" />
                {t("resending")}
              </>
            ) : (
              <>
                <Mail className="mr-1.5 h-4 w-4" />
                {t("resend")}
              </>
            )}
          </Button>
        )}
      </div>
    </div>
  );
}
