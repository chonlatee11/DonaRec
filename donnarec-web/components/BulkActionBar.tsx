"use client";

import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";

interface BulkActionBarProps {
  selectedCount: number;
  /** True when the selection includes >=1 not-yet-keyed row */
  canMark: boolean;
  /** True when the selection includes >=1 already-keyed row */
  canUnmark: boolean;
  isPending: boolean;
  onMark: () => void;
  onUnmark: () => void;
  onClear: () => void;
}

/**
 * BulkActionBar — Screen 7 Tab B: appears when >=1 row is selected
 * (48px height, FR-31/D-67). Every mark/unmark writes one audit row per
 * record server-side (05-04) — no additional confirmation dialog is
 * required here (reversible boolean toggle, not a PII-disclosure event,
 * unlike Export's ExportConfirmDialog).
 *
 * Accessibility Contract: selection-count text is aria-live="polite" so it
 * announces without needing focus to move.
 */
export function BulkActionBar({
  selectedCount,
  canMark,
  canUnmark,
  isPending,
  onMark,
  onUnmark,
  onClear,
}: BulkActionBarProps) {
  const t = useTranslations("aging");

  if (selectedCount === 0) return null;

  return (
    <div className="flex min-h-[48px] items-center gap-3 rounded-lg border border-slate-200 bg-slate-50 px-4 py-2">
      <span className="text-[14px] font-medium text-slate-700" aria-live="polite">
        {t("selectionCount", { n: selectedCount })}
      </span>
      <div className="flex flex-1 flex-wrap justify-end gap-2">
        <Button
          type="button"
          size="sm"
          className="min-h-[44px] bg-blue-600 text-white hover:bg-blue-700"
          disabled={!canMark || isPending}
          onClick={onMark}
        >
          {t("markKeyed")}
        </Button>
        <Button
          type="button"
          size="sm"
          variant="outline"
          className="min-h-[44px]"
          disabled={!canUnmark || isPending}
          onClick={onUnmark}
        >
          {t("unmarkKeyed")}
        </Button>
        <Button
          type="button"
          size="sm"
          variant="ghost"
          className="min-h-[44px] text-slate-600"
          disabled={isPending}
          onClick={onClear}
        >
          {t("clearSelection")}
        </Button>
      </div>
    </div>
  );
}
