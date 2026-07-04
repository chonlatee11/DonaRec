"use client";

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import Link from "next/link";
import { Button } from "@/components/ui/button";
import { SoDBlockedAlert } from "@/components/SoDBlockedAlert";
import { ReviewReasonDialog } from "@/components/ReviewReasonDialog";
import { CancelDialog } from "@/components/CancelDialog";
import { useToast } from "@/hooks/use-toast";
import type { DonationDetail, CancelDonationRequest } from "@/lib/donations";

interface ReviewActionPanelProps {
  donation: Pick<
    DonationDetail,
    | "id"
    | "status"
    | "viewer_is_creator"
    | "can_approve"
    | "can_return"
    | "can_reject"
    | "can_reveal_pii"
    | "edonation_keyed"
    | "receipt_formatted"
  >;
  /** Server action: approve the pending_review record */
  onApprove: () => Promise<{ error: string } | null>;
  /** Server action: return to Maker with a reason */
  onReturn: (reason: string) => Promise<{ error: string } | null>;
  /** Server action: permanently reject the record */
  onReject: (reason: string) => Promise<{ error: string } | null>;
  /** Server action: void (cancel) an issued receipt */
  onCancel?: (body: CancelDonationRequest) => Promise<{ error?: string } | null>;
  /** Server action: void and create replacement draft */
  onReissue?: (body: CancelDonationRequest) => Promise<{ error?: string; newId?: string } | null>;
}

/**
 * ReviewActionPanel — client component for the Screen 3 right-side action panel.
 *
 * Role × status matrix (UI-SPEC §Screen 3):
 *
 *   Maker / draft (viewer is creator):          Edit button → /donations/:id/edit
 *   Checker / pending_review (OWN):             SoDBlockedAlert only (T-03-31)
 *   Checker / pending_review (NOT own):         อนุมัติ + ตีกลับแก้ไข + ปฏิเสธถาวร
 *   Checker|Admin / issued (can_reveal_pii):    ยกเลิกใบเสร็จ + ยกเลิกและออกใบแทน
 *   All other combinations:                     nothing rendered
 *
 * T-03-31: server is the authority — these controls are UX-layer only.
 * Server actions are passed from [id]/page.tsx (Server Component).
 *
 * Cancel/reissue visibility: inferred from can_reveal_pii (Checker+Admin role indicator)
 * combined with status=issued. Backend enforces RBAC regardless of UI display.
 */
export function ReviewActionPanel({
  donation,
  onApprove,
  onReturn,
  onReject,
  onCancel,
  onReissue,
}: ReviewActionPanelProps) {
  const t = useTranslations();
  const router = useRouter();
  const { toast } = useToast();
  const [isPending, startTransition] = useTransition();

  const [returnOpen, setReturnOpen] = useState(false);
  const [rejectOpen, setRejectOpen] = useState(false);
  const [cancelOpen, setCancelOpen] = useState(false);
  const [reissueOpen, setReissueOpen] = useState(false);

  async function runAction(
    action: () => Promise<{ error: string } | null>,
    successMsg: string
  ) {
    startTransition(async () => {
      const result = await action();
      if (result?.error) {
        toast({
          variant: "destructive",
          title: "เกิดข้อผิดพลาด",
          description: result.error,
        });
      } else {
        toast({ description: successMsg });
        router.refresh();
      }
    });
  }

  const heading = (
    <h2 className="text-[20px] font-semibold text-slate-900">
      {t("detail.actionPanel")}
    </h2>
  );

  // ── Case 1: Maker's own draft → Edit button ────────────────────────────────
  if (donation.viewer_is_creator && donation.status === "draft") {
    return (
      <div className="flex flex-col gap-4">
        {heading}
        <Button
          asChild
          variant="outline"
          className="min-h-[44px] w-full"
        >
          <Link href={`/donations/${donation.id}/edit`}>
            {t("detail.editButton")}
          </Link>
        </Button>
      </div>
    );
  }

  // ── Case 2: SoD blocked (Checker viewing their own pending record) ─────────
  // UI-SPEC: approve/return/reject are ABSENT FROM DOM, not just disabled.
  if (donation.viewer_is_creator && donation.status === "pending_review") {
    return (
      <div className="flex flex-col gap-4">
        {heading}
        <SoDBlockedAlert />
      </div>
    );
  }

  // ── Case 3: Checker actions for pending_review (not own record) ────────────
  if (
    donation.status === "pending_review" &&
    (donation.can_approve || donation.can_return || donation.can_reject)
  ) {
    return (
      <div className="flex flex-col gap-4">
        {heading}

        <div className="flex flex-col gap-3">
          {/* อนุมัติ — accent */}
          {donation.can_approve && (
            <Button
              className="min-h-[44px] w-full bg-blue-600 text-white hover:bg-blue-700"
              disabled={isPending}
              onClick={() => runAction(onApprove, "อนุมัติรายการเรียบร้อยแล้ว")}
            >
              {t("actions.approve")}
            </Button>
          )}

          {/* ตีกลับแก้ไข — outline */}
          {donation.can_return && (
            <Button
              variant="outline"
              className="min-h-[44px] w-full"
              disabled={isPending}
              onClick={() => setReturnOpen(true)}
            >
              {t("actions.returnForEdit")}
            </Button>
          )}

          {/* ปฏิเสธถาวร — destructive */}
          {donation.can_reject && (
            <Button
              variant="destructive"
              className="min-h-[44px] w-full"
              disabled={isPending}
              onClick={() => setRejectOpen(true)}
            >
              {t("actions.reject")}
            </Button>
          )}
        </div>

        {/* Return-for-edit dialog — reversible → Dialog */}
        <ReviewReasonDialog
          open={returnOpen}
          onOpenChange={setReturnOpen}
          variant="return"
          isSubmitting={isPending}
          onConfirm={async (reason) => {
            setReturnOpen(false);
            await runAction(
              () => onReturn(reason),
              "ตีกลับรายการเรียบร้อยแล้ว"
            );
          }}
        />

        {/* Reject dialog — terminal → AlertDialog */}
        <ReviewReasonDialog
          open={rejectOpen}
          onOpenChange={setRejectOpen}
          variant="reject"
          isSubmitting={isPending}
          onConfirm={async (reason) => {
            setRejectOpen(false);
            await runAction(
              () => onReject(reason),
              "ปฏิเสธรายการเรียบร้อยแล้ว"
            );
          }}
        />
      </div>
    );
  }

  // ── Case 4: Issued receipt — Checker/Admin can cancel or reissue ───────────
  // Visibility inferred from can_reveal_pii (= Checker or Admin role).
  // Server enforces RBAC on every cancel/reissue call regardless.
  const canCancelOrReissue =
    donation.status === "issued" && donation.can_reveal_pii;

  if (canCancelOrReissue && (onCancel || onReissue)) {
    return (
      <div className="flex flex-col gap-4">
        {heading}

        <div className="flex flex-col gap-3">
          {/* ยกเลิกใบเสร็จ — destructive */}
          {onCancel && (
            <Button
              variant="destructive"
              className="min-h-[44px] w-full"
              disabled={isPending}
              onClick={() => setCancelOpen(true)}
            >
              {t("actions.cancelReceipt")}
            </Button>
          )}

          {/* ยกเลิกและออกใบแทน — destructive outline */}
          {onReissue && (
            <Button
              variant="outline"
              className="min-h-[44px] w-full border-red-300 text-red-600 hover:bg-red-50 hover:text-red-700"
              disabled={isPending}
              onClick={() => setReissueOpen(true)}
            >
              {t("actions.voidAndReissue")}
            </Button>
          )}
        </div>

        {/* Void (cancel) dialog */}
        {onCancel && (
          <CancelDialog
            open={cancelOpen}
            onOpenChange={setCancelOpen}
            mode="void"
            receiptFormatted={donation.receipt_formatted}
            eDonationKeyed={donation.edonation_keyed}
            isSubmitting={isPending}
            onConfirm={async (body) => {
              const result = await onCancel(body);
              if (!result?.error) {
                startTransition(() => {
                  router.refresh();
                });
                toast({ description: "ยกเลิกใบเสร็จเรียบร้อยแล้ว" });
              }
              return result ?? null;
            }}
          />
        )}

        {/* Void & Reissue dialog */}
        {onReissue && (
          <CancelDialog
            open={reissueOpen}
            onOpenChange={setReissueOpen}
            mode="reissue"
            receiptFormatted={donation.receipt_formatted}
            eDonationKeyed={donation.edonation_keyed}
            isSubmitting={isPending}
            onConfirm={async (body) => {
              const result = await onReissue(body);
              if (!result?.error) {
                startTransition(() => {
                  router.refresh();
                });
                toast({
                  description:
                    "ยกเลิกและสร้างรายการใหม่เรียบร้อยแล้ว — ดูรายการใหม่ในรายการบริจาค",
                });
              }
              return result ?? null;
            }}
          />
        )}
      </div>
    );
  }

  // ── No actions available ───────────────────────────────────────────────────
  return null;
}
