"use client";

import { useTranslations } from "next-intl";
import { cn } from "@/lib/utils";
import type { QueueSource } from "@/lib/donations";

interface ChipConfig {
  value: QueueSource;
  labelKey: "all" | "fromWebsite" | "staffEntered";
}

/** Order per UI-SPEC Screen 11: [ ทั้งหมด ] [ จากเว็บไซต์ ] [ เจ้าหน้าที่บันทึก ] */
const CHIPS: ChipConfig[] = [
  { value: "all", labelKey: "all" },
  { value: "from-website", labelKey: "fromWebsite" },
  { value: "staff-entered", labelKey: "staffEntered" },
];

interface QueueSourceFilterProps {
  active: QueueSource;
  onChange: (source: QueueSource) => void;
}

/**
 * QueueSourceFilter — Screen 11 segmented 3-chip source filter (FR-08, D-77).
 *
 * Not a Select — a segmented control (mirrors Phase 5's AgingStatCards clickable
 * toggle pattern): each chip is a native <button> (inherently keyboard-operable)
 * with aria-pressed reflecting the active source, wrapped in a role="group".
 * Active chip: accent blue (bg-blue-600 text-white); inactive: outline
 * (white bg, slate-200 border, slate-700 text). Default "all".
 */
export function QueueSourceFilter({ active, onChange }: QueueSourceFilterProps) {
  const t = useTranslations("queue.filter");

  return (
    <div
      role="group"
      aria-label={t("ariaLabel")}
      className="flex flex-wrap items-center gap-2"
    >
      {CHIPS.map((chip) => {
        const isActive = active === chip.value;
        return (
          <button
            key={chip.value}
            type="button"
            aria-pressed={isActive}
            onClick={() => onChange(chip.value)}
            className={cn(
              "inline-flex min-h-[44px] items-center rounded-full border px-4 text-[14px] font-medium",
              "transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-600 focus-visible:ring-offset-1",
              isActive
                ? "border-blue-600 bg-blue-600 text-white"
                : "border-slate-200 bg-white text-slate-700 hover:border-slate-300"
            )}
          >
            {t(chip.labelKey)}
          </button>
        );
      })}
    </div>
  );
}
