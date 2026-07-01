"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { Eye, EyeOff } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { RevealPIIDialog } from "@/components/RevealPIIDialog";
import { useToast } from "@/hooks/use-toast";

interface MaskedIdFieldProps {
  /** Masked value from server — always the `x-xxxx-xxxxx-1234` format */
  maskedValue: string;
  /**
   * If true, shows the "เปิดเผยข้อมูล" (Reveal) button.
   * Set to true for Checker and Admin roles (based on can_reveal_pii from API).
   */
  canReveal?: boolean;
  /**
   * Audited server action: calls GET /api/donations/:id/pii.
   * Server writes audit log BEFORE returning plaintext (D-46 / T-03-34).
   * Undefined = reveal feature not wired (e.g. list screen).
   */
  onRevealAction?: () => Promise<{ national_id: string } | { error: string }>;
}

/**
 * MaskedIdField — displays donor national/tax ID in masked format by default.
 *
 * UI-SPEC §Screen 3 / Screen 4:
 *   - Always shows masked value x-xxxx-xxxxx-1234 by default
 *   - Checker/Admin see "เปิดเผยข้อมูล" button (Eye icon, blue outline)
 *   - Clicking "เปิดเผยข้อมูล" opens RevealPIIDialog (Screen 4)
 *   - On confirm: POST to /pii → skeleton loader → replace with plaintext
 *   - After reveal: "ซ่อน" button (EyeOff) + tooltip about audit log
 *   - T-03-34: reveal is session-only (client memory); reload re-masks
 *   - 403 → toast "ไม่มีสิทธิ์เปิดเผยข้อมูลนี้"
 */
export function MaskedIdField({
  maskedValue,
  canReveal = false,
  onRevealAction,
}: MaskedIdFieldProps) {
  const t = useTranslations();
  const { toast } = useToast();

  const [dialogOpen, setDialogOpen] = useState(false);
  const [isLoading, setIsLoading] = useState(false);
  /**
   * Session-only plaintext (T-03-34).
   * Stored in component state — cleared on unmount / page reload.
   */
  const [revealedId, setRevealedId] = useState<string | null>(null);

  const isRevealed = revealedId !== null;

  // ── Confirm reveal ────────────────────────────────────────────────────────

  async function handleConfirmReveal() {
    if (!onRevealAction) {
      setDialogOpen(false);
      return;
    }
    setIsLoading(true);
    const result = await onRevealAction();
    setIsLoading(false);

    if ("error" in result) {
      setDialogOpen(false);
      toast({
        variant: "destructive",
        description: t("errors.piiNoPermission"),
        role: "alert",
      });
      return;
    }

    setRevealedId(result.national_id);
    setDialogOpen(false);
  }

  // ── Hide revealed PII ─────────────────────────────────────────────────────

  function handleHide() {
    setRevealedId(null);
  }

  // ── Render ────────────────────────────────────────────────────────────────

  return (
    <div className="flex items-center gap-3">
      {/* ID value — masked or revealed */}
      {isLoading ? (
        <Skeleton className="h-5 w-40" aria-busy="true" />
      ) : isRevealed && revealedId ? (
        <TooltipProvider>
          <Tooltip>
            <TooltipTrigger asChild>
              <span
                className="font-mono text-[14px] font-normal text-slate-900 tracking-wide cursor-default"
                aria-label="เลขประจำตัว (เปิดเผยแล้ว)"
              >
                {revealedId}
              </span>
            </TooltipTrigger>
            <TooltipContent side="top">
              <p className="text-[13px]">{t("pii.revealTooltip")}</p>
            </TooltipContent>
          </Tooltip>
        </TooltipProvider>
      ) : (
        <span
          className="font-mono text-[14px] font-normal text-slate-900 tracking-wide"
          aria-label="เลขประจำตัว (ซ่อน)"
        >
          {maskedValue}
        </span>
      )}

      {/* Reveal / Hide button — visible only to Checker / Admin */}
      {canReveal && (
        <>
          {isRevealed ? (
            /* "ซ่อน" button — hide the revealed value */
            <Button
              variant="outline"
              size="sm"
              className="min-h-[36px] border-slate-300 text-slate-600 hover:bg-slate-50"
              onClick={handleHide}
              aria-label={t("actions.hidePii")}
            >
              <EyeOff className="mr-1.5 h-3.5 w-3.5" />
              {t("actions.hidePii")}
            </Button>
          ) : (
            /* "เปิดเผยข้อมูล" button — opens the confirm dialog */
            <Button
              variant="outline"
              size="sm"
              className="min-h-[36px] border-blue-600 text-blue-600 hover:bg-blue-50 hover:text-blue-700 focus:ring-blue-600"
              onClick={() => setDialogOpen(true)}
              aria-label={t("actions.revealPii")}
            >
              <Eye className="mr-1.5 h-3.5 w-3.5" />
              {t("actions.revealPii")}
            </Button>
          )}

          {/* Audit-confirmed reveal dialog (Screen 4) */}
          <RevealPIIDialog
            open={dialogOpen}
            onOpenChange={setDialogOpen}
            isLoading={isLoading}
            onConfirm={handleConfirmReveal}
          />
        </>
      )}
    </div>
  );
}
