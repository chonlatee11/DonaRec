"use client";

import { useLocale, useTranslations } from "next-intl";
import { useRouter } from "next/navigation";
import { useTransition } from "react";
import { Globe } from "lucide-react";
import { setLocale } from "@/lib/locale-action";
import { cn } from "@/lib/utils";

/**
 * LocaleSwitcher — client component that toggles between th and en.
 *
 * Interaction:
 *   1. Reads current locale from next-intl context (useLocale)
 *   2. On click: calls setLocale server action to write the `locale` cookie
 *   3. Calls router.refresh() to re-render the server tree with the new locale
 *
 * UI-SPEC §i18n Contract:
 *   "Locale switcher: accessible from navigation header (globe icon + current locale label)"
 *   Default locale: th. Second: en.
 */
export function LocaleSwitcher({ className }: { className?: string }) {
  const locale = useLocale();
  const t = useTranslations("locale");
  const router = useRouter();
  const [isPending, startTransition] = useTransition();

  const nextLocale = locale === "th" ? "en" : "th";
  const nextLabel = nextLocale === "th" ? "TH" : "EN";
  const currentLabel = locale === "th" ? "TH" : "EN";

  const handleSwitch = () => {
    startTransition(async () => {
      await setLocale(nextLocale);
      router.refresh();
    });
  };

  return (
    <button
      type="button"
      onClick={handleSwitch}
      disabled={isPending}
      aria-label={t("switch")}
      title={t("switch")}
      className={cn(
        "inline-flex items-center gap-1.5 rounded-md px-3 py-1.5",
        "text-sm font-medium text-slate-600",
        "border border-slate-200 bg-white",
        "hover:bg-slate-50 hover:text-slate-900",
        "focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-600 focus-visible:ring-offset-2",
        "transition-colors disabled:opacity-50 disabled:cursor-not-allowed",
        // UI-SPEC: 44px min touch target height
        "min-h-[44px]",
        className
      )}
    >
      <Globe
        className="h-4 w-4 shrink-0 text-slate-500"
        aria-hidden="true"
      />
      <span>{currentLabel}</span>
      <span className="text-slate-300" aria-hidden="true">
        /
      </span>
      <span className="text-slate-400">{nextLabel}</span>
    </button>
  );
}
