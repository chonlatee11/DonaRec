"use client";

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import Link from "next/link";
import { Button } from "@/components/ui/button";
import { SoDBlockedAlert } from "@/components/SoDBlockedAlert";
import { ReviewReasonDialog } from "@/components/ReviewReasonDialog";
import { useToast } from "@/hooks/use-toast";
import type { DonationDetail } from "@/lib/donations";

interface ReviewActionPanelProps {
  donation: Pick<
    DonationDetail,
    | "id"
    | "status"
    | "viewer_is_creator"
    | "can_approve"
    | "can_return"
    | "can_reject"
  >;
  /**
   * Server action: approve the pending_review record.
   * Returns null on success or { error } on API error.
   */
  onApprove: () => Promise<{ error: string } | null>;
  /** Server action: return to Maker with a reason */
  onReturn: (reason: string) => Promise<{ error: string } | null>;
  /** Server action: permanently reject the record */
  onReject: (reason: string) => Promise<{ error: string } | null>;
}

/**
 * ReviewActionPanel — client component for the Screen 3 right-side action panel.
 *
 * Role × status matrix (UI-SPEC §Screen 3):
 *
 *   Maker / draft (viewer is creator):   Edit button → /donations/:id/edit
 *   Checker / pending_review (OWN):      SoDBlockedAlert only — NO approve/return/reject in DOM
 *   Checker / pending_review (NOT own):  อนุมัติ + ตีกลับแก้ไข + ปฏิเสธถาวร
 *   All other combinations:             nothing rendered
 *
 * T-03-31: server is the authority — these controls are UX-layer only.
 * Server actions are passed from [id]/page.tsx (Server Component).
 */
export function ReviewActionPanel({
  donation,
  onApprove,
  onReturn,
  onReject,
}: ReviewActionPanelProps) {
  const t = useTranslations();
  const router = useRouter();
  const { toast } = useToast();
  const [isPending, startTransition] = useTransition();

  const [returnOpen, setReturnOpen] = useState(false);
  const [rejectOpen, setRejectOpen] = useState(false);

  async function runAction(
    action: () => Promise<{ error: string } | null>,
    successMsg: string
  ) {
    startTransition(async () => {
      const result = await action();
      if (result?.error) {
        // 403 SoD / 409 status-conflict messages come pre-formatted from lib/api.ts
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
          {/* อนุมัติ — accent (UI-SPEC primary CTA) */}
          {donation.can_approve && (
            <Button
              className="min-h-[44px] w-full bg-blue-600 text-white hover:bg-blue-700"
              disabled={isPending}
              onClick={() =>
                runAction(onApprove, "อนุมัติรายการเรียบร้อยแล้ว")
              }
            >
              {t("actions.approve")}
            </Button>
          )}

          {/* ตีกลับแก้ไข — outline (UI-SPEC) */}
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

          {/* ปฏิเสธถาวร — destructive (UI-SPEC) */}
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

  // ── No actions available ───────────────────────────────────────────────────
  return null;
}
