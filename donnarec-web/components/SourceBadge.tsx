"use client";

import { useTranslations } from "next-intl";
import { Globe, UserCog } from "lucide-react";
import { cn } from "@/lib/utils";

/**
 * Donation source values — must stay in sync with the Go API donations.source
 * column (TEXT CHECK flow_a|flow_b, migration 000015, plan 06-01).
 *   flow_a = staff-entered (back-office, the pre-existing default)
 *   flow_b = submitted by a donor via the public web form (Flow B)
 */
export type DonationSource = "flow_a" | "flow_b";

interface SourceConfig {
  /** Tailwind bg class — UI-SPEC §Color "Source badge tokens" (LOCKED, (app) only) */
  bg: string;
  /** Tailwind text class — UI-SPEC locked values */
  text: string;
  /** Lucide icon component */
  Icon: typeof Globe;
  /** Thai label — used for aria-label (always Thai, per Accessibility Contract) */
  labelTh: string;
  /** i18n key under the `source.*` namespace for the visible label */
  key: "flow_a" | "flow_b";
}

/**
 * UI-SPEC §Color "New in Phase 6 — Source badge tokens" table.
 * A deliberately NEUTRAL blue/slate pair (not the green/amber/red urgency
 * vocabulary) distinguishing Flow A vs Flow B records in the Queue / List.
 * These exact bg/text combinations are LOCKED — do not change without updating UI-SPEC.
 */
const SOURCE_CONFIG: Record<DonationSource, SourceConfig> = {
  flow_b: {
    bg: "bg-blue-50",
    text: "text-blue-700",
    Icon: Globe,
    labelTh: "จากเว็บไซต์",
    key: "flow_b",
  },
  flow_a: {
    bg: "bg-slate-100",
    text: "text-slate-600",
    Icon: UserCog,
    labelTh: "เจ้าหน้าที่บันทึก",
    key: "flow_a",
  },
};

interface SourceBadgeProps {
  source: DonationSource;
  className?: string;
}

/**
 * SourceBadge — wraps the UI-SPEC source → color/icon mapping (FR-08, D-77).
 *
 * The visible label is read from the `source.*` i18n namespace so it follows the
 * active locale (TH primary / EN toggle). The aria-label always carries the full
 * Thai source text (mirrors StatusBadge's always-Thai aria pattern) so screen
 * readers announce the source regardless of display locale.
 *
 * Unknown/legacy source values default to the flow_a (staff-entered) treatment —
 * every pre-existing row backfilled to flow_a (plan 06-01), so this is the safe
 * fallback rather than rendering nothing.
 */
export function SourceBadge({ source, className }: SourceBadgeProps) {
  const t = useTranslations("source");
  const config = SOURCE_CONFIG[source] ?? SOURCE_CONFIG.flow_a;
  const { Icon } = config;

  return (
    <span
      className={cn(
        "inline-flex items-center gap-1 rounded-full px-2.5 py-0.5",
        "text-xs font-medium leading-tight",
        "border border-transparent",
        config.bg,
        config.text,
        className
      )}
      aria-label={`แหล่งที่มา: ${config.labelTh}`}
    >
      <Icon className="h-3 w-3 shrink-0" aria-hidden="true" />
      {t(config.key)}
    </span>
  );
}

/** Re-export the config map for consumers that need to drive styling themselves */
export { SOURCE_CONFIG };
