import Link from "next/link";
import { getTranslations } from "next-intl/server";
import { Settings, FileSpreadsheet, BarChart3 } from "lucide-react";
import { LocaleSwitcher } from "./LocaleSwitcher";
import { SignOutButton } from "./SignOutButton";
import { Toaster } from "@/components/ui/toaster";
import { isAdminViewer, isCheckerOrAdminViewer } from "@/lib/session-role";

/**
 * AppShell — root layout shell for all back-office pages.
 *
 * UI-SPEC §Spacing:
 *   - Sidebar: slate-100 bg, lg (24px) horizontal padding, lg vertical padding
 *   - Main content area: slate-50 bg
 *   - Page-level vertical padding: 3xl (64px) → `py-16` (Tailwind p-unit × 4 = 64px)
 *
 * UI-SPEC §Color:
 *   - Dominant 60%: slate-50 (#F8FAFC) — page/layout shell
 *   - Secondary 30%: slate-100 (#F1F5F9) — sidebar/nav background
 *   - Border: slate-200 (#E2E8F0)
 *
 * This is a Server Component (async). LocaleSwitcher is a Client Component
 * imported here — RSC can freely include Client Component boundaries.
 */
export async function AppShell({
  children,
}: {
  children: React.ReactNode;
}) {
  const t = await getTranslations("nav");
  const tApp = await getTranslations("app");
  // T-04-25: UX-layer hint only — Go's adminGroup RequireRoles(RoleAdmin)
  // (04-07) is the real authority. A stale/forged client hint here can, at
  // worst, show the link to a non-Admin, who then gets a 403 on /admin/settings.
  const isAdmin = await isAdminViewer();
  // T-05-06-UXGATE: UX-layer hint only — Go's edonationGroup
  // RequireAnyRole(Checker,Admin) (05-02) is the real authority.
  const isCheckerOrAdmin = await isCheckerOrAdminViewer();

  return (
    <div className="flex min-h-screen bg-slate-50">
      {/* ── Sidebar / Nav ── */}
      <aside className="flex w-64 flex-col bg-slate-100 border-r border-slate-200 shrink-0">
        {/* Brand */}
        <div className="flex items-center px-6 py-5 border-b border-slate-200">
          <span className="text-xl font-semibold text-slate-900 leading-tight">
            {tApp("title")}
          </span>
        </div>

        {/* Nav links */}
        <nav className="flex-1 px-4 py-6 space-y-1" aria-label="Main navigation">
          <Link
            href="/donations"
            className={[
              "flex items-center gap-2 rounded-md px-3 py-2",
              "text-sm text-slate-700",
              "hover:bg-slate-200 hover:text-slate-900",
              "focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-600",
              // UI-SPEC: 44px min touch target
              "min-h-[44px]",
            ].join(" ")}
          >
            {t("donations")}
          </Link>
          <Link
            href="/queue"
            className={[
              "flex items-center gap-2 rounded-md px-3 py-2",
              "text-sm text-slate-700",
              "hover:bg-slate-200 hover:text-slate-900",
              "focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-600",
              "min-h-[44px]",
            ].join(" ")}
          >
            {t("queue")}
          </Link>
          {/*
           * e-Donation export/aging nav item — Checker/Admin only (Screen 7,
           * D-63, plan 05-06). UX hint only (T-05-06-UXGATE) — Go's
           * edonationGroup RequireAnyRole(Checker,Admin) is the real authority.
           */}
          {isCheckerOrAdmin && (
            <Link
              href="/e-donation"
              className={[
                "flex items-center gap-2 rounded-md px-3 py-2",
                "text-sm text-slate-700",
                "hover:bg-slate-200 hover:text-slate-900",
                "focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-600",
                "min-h-[44px]",
              ].join(" ")}
            >
              <FileSpreadsheet className="h-4 w-4" />
              {t("eDonation")}
            </Link>
          )}
          {/*
           * Reports nav item — always visible (Screen 8, D-71, plan 05-07).
           * No RBAC gate: zero PII in the report, transparent to all staff.
           */}
          <Link
            href="/reports"
            className={[
              "flex items-center gap-2 rounded-md px-3 py-2",
              "text-sm text-slate-700",
              "hover:bg-slate-200 hover:text-slate-900",
              "focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-600",
              "min-h-[44px]",
            ].join(" ")}
          >
            <BarChart3 className="h-4 w-4" />
            {t("reports")}
          </Link>
          {/*
           * Settings nav item — Admin-only (Screen 6, D-58, plan 04-08).
           * UI-SPEC Screen 6: "Add a new nav link in AppShell's sidebar ...
           * rendered only when the signed-in user's role is Admin."
           */}
          {isAdmin && (
            <Link
              href="/admin/settings"
              className={[
                "flex items-center gap-2 rounded-md px-3 py-2",
                "text-sm text-slate-700",
                "hover:bg-slate-200 hover:text-slate-900",
                "focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-600",
                "min-h-[44px]",
              ].join(" ")}
            >
              <Settings className="h-4 w-4" />
              {t("settings")}
            </Link>
          )}
        </nav>
      </aside>

      {/* ── Main area ── */}
      <div className="flex flex-1 flex-col min-w-0">
        {/* Header */}
        <header className="flex h-14 shrink-0 items-center justify-end gap-4 border-b border-slate-200 bg-white px-6">
          <SignOutButton />
          <LocaleSwitcher />
        </header>

        {/* Page content — 3xl (64px) vertical padding per UI-SPEC Spacing */}
        <main className="flex-1 py-16 px-6">
          {children}
        </main>
      </div>
      {/* Toaster — required for ReviewActionPanel approve/return/reject success/error feedback */}
      <Toaster />
    </div>
  );
}
