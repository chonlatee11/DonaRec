import { cn } from "@/lib/utils";

/**
 * Donation status values — must stay in sync with Go API DonationStatus enum.
 * UI-SPEC §Color "Status badge color tokens"
 */
export type DonationStatus =
  | "draft"
  | "pending_review"
  | "issued"
  | "rejected"
  | "cancelled";

interface StatusConfig {
  /** Tailwind bg class — UI-SPEC locked values */
  bg: string;
  /** Tailwind text class — UI-SPEC locked values */
  text: string;
  /** Thai label — used for aria-label */
  labelTh: string;
  /** English label */
  labelEn: string;
}

/**
 * UI-SPEC §Color "Status badge color tokens" table.
 * These exact bg/text combinations are LOCKED — do not change without updating UI-SPEC.
 */
const STATUS_CONFIG: Record<DonationStatus, StatusConfig> = {
  draft: {
    bg: "bg-slate-100",
    text: "text-slate-600",
    labelTh: "ร่าง",
    labelEn: "Draft",
  },
  pending_review: {
    bg: "bg-amber-50",
    text: "text-amber-600",
    labelTh: "รอตรวจสอบ",
    labelEn: "Pending Review",
  },
  issued: {
    bg: "bg-green-50",
    text: "text-green-600",
    labelTh: "ออกใบเสร็จแล้ว",
    labelEn: "Issued",
  },
  rejected: {
    bg: "bg-red-50",
    text: "text-red-600",
    labelTh: "ปฏิเสธ",
    labelEn: "Rejected",
  },
  cancelled: {
    bg: "bg-slate-50",
    text: "text-slate-500",
    labelTh: "ยกเลิก",
    labelEn: "Cancelled",
  },
};

interface StatusBadgeProps {
  status: DonationStatus;
  /**
   * Locale determines which label text to render inside the badge.
   * Defaults to "th" per UI-SPEC i18n contract (Thai is primary locale).
   */
  locale?: "th" | "en";
  className?: string;
}

/**
 * StatusBadge — wraps UI-SPEC status → color mapping.
 *
 * Accessibility (UI-SPEC Accessibility Contract):
 *   aria-label="สถานะ: {Thai label}" always — so screen readers always
 *   get the Thai status name regardless of display locale.
 *
 * Cancelled status (UI-SPEC): strikethrough is applied on the *receipt number*
 * cell in the table (not on the badge itself). The badge only shows the
 * cancelled colour. The caller is responsible for the strikethrough on numbers.
 */
export function StatusBadge({
  status,
  locale = "th",
  className,
}: StatusBadgeProps) {
  const config = STATUS_CONFIG[status];
  const displayLabel = locale === "en" ? config.labelEn : config.labelTh;

  return (
    <span
      className={cn(
        "inline-flex items-center rounded-full px-2.5 py-0.5",
        "text-xs font-medium leading-tight",
        "border border-transparent",
        config.bg,
        config.text,
        className
      )}
      // UI-SPEC Accessibility Contract: always Thai label in aria-label
      aria-label={`สถานะ: ${config.labelTh}`}
      role="status"
    >
      {displayLabel}
    </span>
  );
}

/** Re-export the config map for consumers that need to drive styling themselves */
export { STATUS_CONFIG };
