"use client";

import { useTranslations } from "next-intl";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";

interface KeyedStatusBadgeProps {
  keyed: boolean;
  className?: string;
}

/**
 * KeyedStatusBadge — wraps shadcn Badge with keyed/not-keyed color mapping
 * (FR-31/D-67). UI-SPEC §Color "New in Phase 5 — Keyed-status badge token" —
 * LOCKED bg/text values, deliberately distinct from the 3 aging-bucket hues
 * so the two badges shown side-by-side in one table row are visually
 * distinguishable at a glance (reuses Phase 4's Info banner blue).
 */
export function KeyedStatusBadge({ keyed, className }: KeyedStatusBadgeProps) {
  const t = useTranslations("aging");
  const labelTh = keyed ? "คีย์เข้า e-Donation แล้ว" : "ยังไม่คีย์";

  return (
    <Badge
      className={cn(
        "border-transparent font-medium",
        keyed ? "bg-blue-50 text-blue-700" : "bg-slate-100 text-slate-600",
        className
      )}
      aria-label={t("ariaKeyedStatusLabel", { status: labelTh })}
    >
      {keyed ? t("keyedStatusKeyedBadge") : t("keyedStatusNotKeyedBadge")}
    </Badge>
  );
}
