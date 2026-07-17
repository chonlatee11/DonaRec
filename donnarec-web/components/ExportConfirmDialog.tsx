"use client";

import { AlertTriangle } from "lucide-react";
import { useTranslations } from "next-intl";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";

interface ExportConfirmDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /**
   * Number of donors in the current filtered result set, when known.
   * Null when the count preview could not be derived (D-66 keyed_status =
   * "all"/"keyed" — the Aging endpoint only returns unkeyed rows, see
   * ExportPanel's countPreview derivation) — the warning banner falls back
   * to a count-free copy variant in that case rather than showing a
   * fabricated number.
   */
  count: number | null;
  /** True while the export download is in-flight (D-74 spirit: block a second concurrent request) */
  isSubmitting?: boolean;
  onConfirm: () => void;
}

/**
 * ExportConfirmDialog — audited PII-heavy export confirmation (Screen 7 Tab
 * A, D-64). Parallel structure to RevealPIIDialog/CancelDialog's AlertDialog
 * pattern: amber-50 warning banner (not destructive red — export is
 * reversible/non-data-destroying, D-74) reminding the exporter the file
 * contains full plaintext 13-digit national/tax IDs and must be handled/
 * deleted securely (T-05-06-FILECUSTODY).
 */
export function ExportConfirmDialog({
  open,
  onOpenChange,
  count,
  isSubmitting = false,
  onConfirm,
}: ExportConfirmDialogProps) {
  const t = useTranslations("eDonationExport.confirmDialog");
  const tActions = useTranslations("actions");

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent className="max-w-lg">
        <AlertDialogHeader>
          <AlertDialogTitle>{t("title")}</AlertDialogTitle>
          <AlertDialogDescription asChild>
            <div
              className="flex items-start gap-2.5 rounded-md border border-amber-200 bg-amber-50 px-4 py-3"
              role="alert"
            >
              <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-amber-600" />
              <p className="text-[14px] leading-relaxed text-amber-700">
                {count !== null ? t("warning", { n: count }) : t("warningNoCount")}
              </p>
            </div>
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={isSubmitting}>
            {tActions("cancel")}
          </AlertDialogCancel>
          <AlertDialogAction
            onClick={onConfirm}
            disabled={isSubmitting}
            aria-busy={isSubmitting}
            className="bg-blue-600 text-white hover:bg-blue-700 focus:ring-blue-600"
          >
            {t("confirm")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
