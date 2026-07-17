"use client";

import { useTranslations } from "next-intl";
import { cn } from "@/lib/utils";
import type { AgingBucket } from "@/lib/edonation";

interface BucketCardConfig {
  text: string;
  labelTh: string;
  labelKey: "bucketNotDue" | "bucketNearDue" | "bucketOverdue";
}

const BUCKET_ORDER: AgingBucket[] = ["not_due", "near_due", "overdue"];

const BUCKET_CARD_CONFIG: Record<AgingBucket, BucketCardConfig> = {
  not_due: { text: "text-green-600", labelTh: "ยังไม่ถึงกำหนด", labelKey: "bucketNotDue" },
  near_due: { text: "text-amber-600", labelTh: "ใกล้ครบกำหนด", labelKey: "bucketNearDue" },
  overdue: { text: "text-red-600", labelTh: "เกินกำหนด", labelKey: "bucketOverdue" },
};

interface AgingStatCardsProps {
  counts: Partial<Record<AgingBucket, number>>;
  /** Active bucket filter, or null when showing all unkeyed buckets together */
  activeBucket: AgingBucket | null;
  onToggle: (bucket: AgingBucket) => void;
}

/**
 * AgingStatCards — Screen 7 Tab B: 3 clickable bucket-count cards
 * (not_due/near_due/overdue) acting as table filter toggles (FR-31/D-68).
 *
 * Accessibility Contract: each card is a native <button> (inherently
 * keyboard-operable via Enter/Space and implicitly role="button"/
 * tabIndex=0 — a stronger a11y guarantee than adding those attributes to a
 * non-interactive element) with aria-pressed reflecting the active bucket
 * filter toggle state.
 *
 * Cards only ever show unkeyed records' bucket counts — once a record is
 * marked keyed it drops out of all three buckets (the aging view's entire
 * purpose is surfacing what's NOT yet done).
 */
export function AgingStatCards({ counts, activeBucket, onToggle }: AgingStatCardsProps) {
  const t = useTranslations("aging");

  return (
    <div className="grid grid-cols-1 gap-6 sm:grid-cols-3">
      {BUCKET_ORDER.map((bucket) => {
        const config = BUCKET_CARD_CONFIG[bucket];
        const isActive = activeBucket === bucket;
        return (
          <button
            key={bucket}
            type="button"
            aria-pressed={isActive}
            aria-label={t("ariaBucketFilterLabel", { bucket: config.labelTh })}
            onClick={() => onToggle(bucket)}
            className={cn(
              "flex flex-col items-start gap-1 rounded-lg border bg-white p-4 text-left",
              "min-h-[44px] transition-colors",
              "focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-600",
              isActive
                ? "border-blue-600 ring-1 ring-blue-600"
                : "border-slate-200 hover:border-slate-300"
            )}
          >
            <span className="text-[14px] text-slate-600">{t(config.labelKey)}</span>
            <span className={cn("text-[28px] font-semibold leading-tight", config.text)}>
              {counts[bucket] ?? 0}
            </span>
          </button>
        );
      })}
    </div>
  );
}
