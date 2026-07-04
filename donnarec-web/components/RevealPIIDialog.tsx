"use client";

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

interface RevealPIIDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** True while the /pii server call is in-flight (shows skeleton) */
  isLoading?: boolean;
  /**
   * Called when user clicks "ยืนยันการเปิดเผย".
   * The caller is responsible for calling the audited reveal endpoint
   * and updating the displayed value.
   */
  onConfirm: () => void;
}

/**
 * RevealPIIDialog — audited confirmation dialog for PII reveal (Screen 4).
 *
 * UI-SPEC §Screen 4:
 *   - AlertDialog (terminal/destructive action)
 *   - Title: "เปิดเผยเลขประจำตัวผู้เสียภาษี / เลขบัตรประชาชน"
 *   - Body: audit log warning
 *   - Confirm: "ยืนยันการเปิดเผย" (accent)
 *   - Cancel: "ยกเลิก" (outline)
 *
 * T-03-34: reveal is session-only (client memory) — server audits every call.
 * The caller (MaskedIdField) controls the session state and passes the
 * audited server action via onConfirm.
 */
export function RevealPIIDialog({
  open,
  onOpenChange,
  isLoading = false,
  onConfirm,
}: RevealPIIDialogProps) {
  const t = useTranslations();

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>
            {t("dialogs.revealPii.title")}
          </AlertDialogTitle>
          <AlertDialogDescription>
            {isLoading ? (
              // Inline skeleton (span, not the block <div> Skeleton): AlertDialogDescription
              // renders a <p>, and a <div> descendant is invalid HTML → hydration error.
              <span
                className="inline-block h-4 w-3/4 animate-pulse rounded-md bg-muted align-middle"
                aria-busy="true"
              />
            ) : (
              t("dialogs.revealPii.body")
            )}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={isLoading}>
            {t("actions.cancel")}
          </AlertDialogCancel>
          <AlertDialogAction
            onClick={onConfirm}
            disabled={isLoading}
            className="bg-blue-600 text-white hover:bg-blue-700 focus:ring-blue-600"
          >
            {isLoading ? "กำลังดึงข้อมูล..." : t("actions.confirmReveal")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
