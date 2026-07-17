"use client";

import { useTranslations } from "next-intl";
import { LocaleSwitcher } from "@/components/LocaleSwitcher";

/**
 * PublicHeader — minimal, sticky/translucent header for the donor-facing
 * (public) surface (Screen 9, 06-UI-SPEC). Unlike (app)'s AppShell it has NO
 * sidebar, NO SignOutButton, and NO role checks — it renders for a fully
 * unauthenticated first-time visitor.
 *
 * Layout: a pine-filled rounded brand mark + the hospital name as STATIC text
 * (Trirong 20/600 — deliberately not a link, since there is no authenticated
 * home to return to) on the left, and the reused LocaleSwitcher on the right.
 * The switcher and every label inside .theme-public are restyled to the warm
 * palette purely by the scope, no prop changes.
 *
 * Responsive heights per the 06-UI-SPEC consolidated token table:
 * 56px (base) / 64px (sm) / 70px (md+). Sticky + backdrop-blur at every
 * breakpoint so the language toggle stays reachable on a long scrolling form.
 */
export function PublicHeader() {
  const t = useTranslations("publicDonation");

  return (
    <header
      className="sticky top-0 z-40 border-b border-border backdrop-blur-[10px]"
      style={{ backgroundColor: "rgba(245, 243, 236, 0.82)" }}
    >
      <div className="mx-auto flex h-14 max-w-[960px] items-center justify-between px-4 sm:h-16 md:h-[70px] md:px-6">
        <div className="flex items-center gap-3">
          <span
            aria-hidden="true"
            className="flex h-8 w-8 items-center justify-center rounded-[9px] bg-primary text-[16px] text-primary-foreground sm:h-9 sm:w-9 sm:rounded-[10px] md:h-[38px] md:w-[38px] md:rounded-[11px]"
            style={{ fontFamily: "var(--font-trirong)" }}
          >
            บ
          </span>
          <span
            className="text-[20px] font-semibold text-primary"
            style={{ fontFamily: "var(--font-trirong)" }}
          >
            {t("brandName")}
          </span>
        </div>
        <LocaleSwitcher />
      </div>
    </header>
  );
}
