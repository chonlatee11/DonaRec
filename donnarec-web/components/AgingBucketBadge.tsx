"use client";

import { useTranslations } from "next-intl";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import type { AgingBucket } from "@/lib/edonation";

interface BucketConfig {
  bg: string;
  text: string;
  /** Thai label — used for aria-label regardless of display locale */
  labelTh: string;
}

/**
 * UI-SPEC §Color "New in Phase 5 — Aging bucket badge tokens" — LOCKED
 * bg/text values, reused from the existing donation-status badge hues
 * (green=good, amber=warning, red=urgent) applied to a new semantic
 * (time-to-deadline).
 */
const BUCKET_CONFIG: Record<AgingBucket, BucketConfig> = {
  not_due: { bg: "bg-green-50", text: "text-green-600", labelTh: "ยังไม่ถึงกำหนด" },
  near_due: { bg: "bg-amber-50", text: "text-amber-600", labelTh: "ใกล้ครบกำหนด" },
  overdue: { bg: "bg-red-50", text: "text-red-600", labelTh: "เกินกำหนด" },
};

interface AgingBucketBadgeProps {
  bucket: AgingBucket;
  className?: string;
}

/**
 * AgingBucketBadge — wraps shadcn Badge with bucket→color mapping
 * (green/amber/red, FR-31/D-68). Accessibility Contract: aria-label with
 * full Thai text regardless of display locale (same convention as
 * StatusBadge).
 */
export function AgingBucketBadge({ bucket, className }: AgingBucketBadgeProps) {
  const t = useTranslations("aging");
  const config = BUCKET_CONFIG[bucket];
  const labelKey =
    bucket === "not_due"
      ? "bucketNotDue"
      : bucket === "near_due"
        ? "bucketNearDue"
        : "bucketOverdue";

  return (
    <Badge
      className={cn(
        "border-transparent font-medium",
        config.bg,
        config.text,
        className
      )}
      aria-label={t("ariaBucketLabel", { bucket: config.labelTh })}
    >
      {t(labelKey)}
    </Badge>
  );
}
