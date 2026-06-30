"use client";

import { useTranslations } from "next-intl";
import { Eye } from "lucide-react";
import { Button } from "@/components/ui/button";

interface MaskedIdFieldProps {
  /** Masked value from server — always the `x-xxxx-xxxxx-1234` format */
  maskedValue: string;
  /**
   * If true, shows the "เปิดเผยข้อมูล" (Reveal) button.
   * Set to true for Checker and Admin roles (based on can_reveal_pii from API).
   * Full AlertDialog reveal flow is implemented in plan 03-08.
   */
  canReveal?: boolean;
  /**
   * Called when the user clicks "เปิดเผยข้อมูล" and confirms the reveal dialog.
   * Implemented fully in 03-08; here it is a placeholder prop.
   */
  onReveal?: () => void;
}

/**
 * MaskedIdField — displays donor national/tax ID in masked format by default.
 *
 * UI-SPEC §Screen 3: national ID row always shows `x-xxxx-xxxxx-1234` by default.
 * T-03-32: plaintext only via audited reveal endpoint (03-06 / 03-08).
 *
 * Masked value rendered in font-mono 14px per UI-SPEC Typography.
 * `aria-label="เลขประจำตัว (ซ่อน)"` per UI-SPEC Accessibility Contract.
 *
 * The full reveal AlertDialog flow (Screen 4) is wired in 03-08.
 * This component exposes the `onReveal` prop and renders the masked state + button.
 */
export function MaskedIdField({
  maskedValue,
  canReveal = false,
  onReveal,
}: MaskedIdFieldProps) {
  const t = useTranslations();

  return (
    <div className="flex items-center gap-3">
      {/* Masked value — font-mono 14px per UI-SPEC Typography */}
      <span
        className="font-mono text-[14px] font-normal text-slate-900 tracking-wide"
        aria-label="เลขประจำตัว (ซ่อน)"
      >
        {maskedValue}
      </span>

      {/* Reveal button — visible only to Checker / Admin (can_reveal_pii = true) */}
      {canReveal && (
        <Button
          variant="outline"
          size="sm"
          className="min-h-[36px] border-blue-600 text-blue-600 hover:bg-blue-50 hover:text-blue-700 focus:ring-blue-600"
          onClick={onReveal}
          aria-label={t("actions.revealPii")}
          /* Tooltip note: full audit tooltip rendered in 03-08 after reveal */
        >
          <Eye className="mr-1.5 h-3.5 w-3.5" />
          {t("actions.revealPii")}
        </Button>
      )}
    </div>
  );
}
