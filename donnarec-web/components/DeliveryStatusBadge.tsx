import { cn } from "@/lib/utils";

/**
 * Email delivery status values (Screen 3b, FR-27).
 *
 * "pending" is a FE-only synthetic state — the Go `email_delivery` table only
 * ever stores 'sent' | 'failed' | 'no_email' (one row per attempt). When the
 * donation is issued/cancelled but `email_delivery` is still null (the outbox
 * worker, 04-05, has not recorded any attempt yet), the FE renders "pending"
 * to match the UI-SPEC's "pending / retrying" badge token — it also doubles
 * as the visual for an auto-retry-in-progress attempt, since the worker does
 * not expose a separate "retrying" signal to the FE.
 */
export type EmailDeliveryStatus = "pending" | "sent" | "failed" | "no_email";

interface DeliveryStatusConfig {
  /** Tailwind bg class — UI-SPEC "Email delivery status badge color tokens" table */
  bg: string;
  /** Tailwind text class — UI-SPEC locked values */
  text: string;
  /** Thai label — used for aria-label (always Thai, per StatusBadge precedent) */
  labelTh: string;
  /** English label */
  labelEn: string;
}

/**
 * UI-SPEC §Color "Email delivery status badge color tokens" table.
 * These exact bg/text combinations are LOCKED — do not change without updating UI-SPEC.
 */
const DELIVERY_STATUS_CONFIG: Record<EmailDeliveryStatus, DeliveryStatusConfig> = {
  pending: {
    bg: "bg-amber-50",
    text: "text-amber-600",
    labelTh: "กำลังส่ง / รอส่งซ้ำ",
    labelEn: "Pending / Retrying",
  },
  sent: {
    bg: "bg-green-50",
    text: "text-green-600",
    labelTh: "ส่งสำเร็จ",
    labelEn: "Sent",
  },
  failed: {
    bg: "bg-red-50",
    text: "text-red-600",
    labelTh: "ส่งไม่สำเร็จ",
    labelEn: "Failed",
  },
  no_email: {
    bg: "bg-slate-100",
    text: "text-slate-600",
    labelTh: "ไม่มีอีเมลผู้บริจาค",
    labelEn: "No Email",
  },
};

interface DeliveryStatusBadgeProps {
  status: EmailDeliveryStatus;
  /**
   * Locale determines which label text to render inside the badge.
   * Defaults to "th" per UI-SPEC i18n contract (Thai is primary locale).
   */
  locale?: "th" | "en";
  className?: string;
}

/**
 * DeliveryStatusBadge — wraps the UI-SPEC email-delivery-status → color mapping.
 * Parallel structure to StatusBadge (Phase 3) — same span/role/aria-label shape.
 *
 * Accessibility (UI-SPEC Accessibility Contract):
 *   aria-label="สถานะการส่งอีเมล: {Thai label}" always, regardless of display locale.
 */
export function DeliveryStatusBadge({
  status,
  locale = "th",
  className,
}: DeliveryStatusBadgeProps) {
  const config = DELIVERY_STATUS_CONFIG[status];
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
      aria-label={`สถานะการส่งอีเมล: ${config.labelTh}`}
      role="status"
    >
      {displayLabel}
    </span>
  );
}

/** Re-export the config map for consumers that need to drive styling themselves */
export { DELIVERY_STATUS_CONFIG };
